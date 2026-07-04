"""
OpenTelemetry wiring for the Dexiask engine.

Exports OTLP traces, metrics, and logs to an OTLP Collector. The job
span is parented to the Go backend's `engine.POST` client span (FastAPI extracts
the inbound W3C traceparent), giving one correlated trace per chat turn.

Everything here is a no-op until ``init_observability`` installs real providers,
so the rest of the engine can call these helpers unconditionally. Instruments and
tracer are created from the global proxy providers, which forward to the real SDK
once configured.
"""
from __future__ import annotations

import logging
import random
import sys
import time
from collections.abc import Iterator
from contextlib import contextmanager
from typing import TYPE_CHECKING, Any

from opentelemetry import metrics, trace

if TYPE_CHECKING:
    from engine_core.models.events import Usage
from opentelemetry.trace import Span, Status, StatusCode

# Max bytes of full content attached to a span when the sampler fires.
_MAX_CONTENT_BYTES = 16 * 1024

# Proxy tracer/meter — forward to the real providers once init runs.
_tracer = trace.get_tracer("dexiask-engine")
_meter = metrics.get_meter("dexiask-engine")

# Metric instruments. Names map to Prometheus series via the Collector:
#   dexiask.engine.tool.calls      → dexiask_engine_tool_calls_total
#   dexiask.engine.tool.duration   → dexiask_engine_tool_duration_seconds_*
#   dexiask.llm.tokens             → dexiask_llm_tokens_total
#   dexiask.llm.cost.usd           → dexiask_llm_cost_usd_total
#   dexiask.engine.jobs            → dexiask_engine_jobs_total
#   dexiask.engine.job.duration    → dexiask_engine_job_duration_seconds_*
#   dexiask.engine.turns           → dexiask_engine_turns_*
_tool_calls = _meter.create_counter("dexiask.engine.tool.calls", unit="1", description="Tool invocations")
_tool_duration = _meter.create_histogram("dexiask.engine.tool.duration", unit="s", description="Tool duration")
_llm_tokens = _meter.create_counter("dexiask.llm.tokens", unit="1", description="LLM tokens by model and kind")
_llm_cost = _meter.create_counter("dexiask.llm.cost.usd", unit="1", description="LLM cost in USD by model")
_jobs = _meter.create_counter("dexiask.engine.jobs", unit="1", description="Jobs by terminal status")
_job_duration = _meter.create_histogram("dexiask.engine.job.duration", unit="s", description="Job duration")
_turns = _meter.create_histogram("dexiask.engine.turns", unit="1", description="Turns per job")

_content_rate = 0.0
_initialized = False
_logging_configured = False
_providers: list[Any] = []


def get_tracer() -> trace.Tracer:
    """Return the engine tracer (proxy until init, real SDK after)."""
    return _tracer


# Per-engine log label, e.g. "[claude-engine]". Set by init_observability before
# the stderr handler is created.
_engine_label = "engine"


def configure_logging(level: int = logging.INFO) -> None:
    """Install a stderr handler on the root logger (idempotent). The OTLP log
    handler is added later by init_observability when enabled."""
    global _logging_configured
    if _logging_configured:
        return
    root = logging.getLogger()
    root.setLevel(level)
    handler = logging.StreamHandler(sys.stderr)
    handler.setFormatter(logging.Formatter(f"[{_engine_label}-engine] %(message)s"))
    root.addHandler(handler)
    _logging_configured = True


def init_observability(cfg: Any, engine_name: str | None = None) -> None:
    """Install OTLP trace/metric/log providers (idempotent). Reads the standard
    OTEL_EXPORTER_OTLP_ENDPOINT; a no-op when disabled or unset. ``engine_name``
    (e.g. "claude") sets the log prefix and must be passed before logging is first
    configured."""
    global _initialized, _content_rate, _engine_label
    if engine_name:
        _engine_label = engine_name
    configure_logging()
    if _initialized:
        return

    enabled = (not getattr(cfg, "otel_sdk_disabled", False)) and bool(
        getattr(cfg, "otel_exporter_otlp_endpoint", "")
    )
    if not enabled:
        logging.getLogger("dexiask-engine").info(
            "observability: disabled (set OTEL_EXPORTER_OTLP_ENDPOINT to enable)"
        )
        _initialized = True
        return

    # Imported lazily so the SDK is only required when actually enabled.
    from opentelemetry._logs import set_logger_provider
    from opentelemetry.exporter.otlp.proto.http._log_exporter import OTLPLogExporter
    from opentelemetry.exporter.otlp.proto.http.metric_exporter import OTLPMetricExporter
    from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
    from opentelemetry.sdk._logs import LoggerProvider, LoggingHandler
    from opentelemetry.sdk._logs.export import BatchLogRecordProcessor
    from opentelemetry.sdk.metrics import MeterProvider
    from opentelemetry.sdk.metrics.export import PeriodicExportingMetricReader
    from opentelemetry.sdk.resources import Resource
    from opentelemetry.sdk.trace import TracerProvider
    from opentelemetry.sdk.trace.export import BatchSpanProcessor
    from opentelemetry.sdk.trace.sampling import ParentBased, TraceIdRatioBased

    _content_rate = float(getattr(cfg, "otel_content_sample_rate", 0.05))
    base = cfg.otel_exporter_otlp_endpoint.rstrip("/")
    resource = Resource.create(
        {
            "service.name": getattr(cfg, "otel_service_name", "dexiask-engine"),
            "service.version": "2.0.0",
        }
    )

    tp = TracerProvider(resource=resource, sampler=ParentBased(TraceIdRatioBased(1.0)))
    tp.add_span_processor(BatchSpanProcessor(OTLPSpanExporter(endpoint=f"{base}/v1/traces")))
    trace.set_tracer_provider(tp)

    reader = PeriodicExportingMetricReader(OTLPMetricExporter(endpoint=f"{base}/v1/metrics"))
    mp = MeterProvider(resource=resource, metric_readers=[reader])
    metrics.set_meter_provider(mp)

    lp = LoggerProvider(resource=resource)
    lp.add_log_record_processor(BatchLogRecordProcessor(OTLPLogExporter(endpoint=f"{base}/v1/logs")))
    set_logger_provider(lp)
    logging.getLogger().addHandler(LoggingHandler(level=logging.INFO, logger_provider=lp))

    _providers.extend([tp, mp, lp])
    _initialized = True
    logging.getLogger("dexiask-engine").info(
        f"observability: initialized → {base} (service={resource.attributes['service.name']})"
    )


def instrument_fastapi(app: Any) -> None:
    """Instrument a FastAPI app so POST /v1/jobs extracts the inbound traceparent
    and starts a server span. Safe to call when disabled (becomes a cheap no-op)."""
    try:
        from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor

        FastAPIInstrumentor.instrument_app(app)
    except Exception as e:  # pragma: no cover — instrumentation is best-effort
        logging.getLogger("dexiask-engine").warning(f"observability: FastAPI instrument failed: {e}")


def shutdown_observability() -> None:
    """Flush and shut down all providers (called on server shutdown)."""
    for p in _providers:
        try:
            p.shutdown()
        except Exception:  # pragma: no cover
            pass


# ---------------------------------------------------------------------------
# Content sampling
# ---------------------------------------------------------------------------

def should_capture_content(is_error: bool) -> bool:
    """Always capture on error; otherwise sample at the configured rate."""
    if is_error:
        return True
    if _content_rate <= 0:
        return False
    if _content_rate >= 1:
        return True
    return random.random() < _content_rate


def set_span_content(span: Span | None, key: str, content: str, is_error: bool) -> None:
    """Record <key>.length always; attach (truncated) full content only when sampled."""
    if span is None or not span.is_recording():
        return
    content = content or ""
    span.set_attribute(f"{key}.length", len(content))
    if content and should_capture_content(is_error):
        span.set_attribute(key, content[:_MAX_CONTENT_BYTES])


# ---------------------------------------------------------------------------
# Tool instrumentation
# ---------------------------------------------------------------------------

@contextmanager
def tool_span(name: str, **attrs: Any) -> Iterator[Span]:
    """Wrap a tool invocation in a span and record tool-call metrics."""
    start = time.perf_counter()
    status = "success"
    with _tracer.start_as_current_span(f"tool.{name}") as span:
        span.set_attribute("tool.name", name)
        for k, v in attrs.items():
            if v is not None:
                span.set_attribute(k, v)
        try:
            yield span
        except Exception as e:
            status = "error"
            span.record_exception(e)
            span.set_status(Status(StatusCode.ERROR))
            raise
        finally:
            _tool_calls.add(1, {"tool": name, "status": status})
            _tool_duration.record(time.perf_counter() - start, {"tool": name})


# ---------------------------------------------------------------------------
# Result / job metrics
# ---------------------------------------------------------------------------

def _usage_get(usage: Any, key: str) -> int:
    """Read a token count from a usage dict or object, defaulting to 0.

    Supports dotted paths (e.g. ``input_tokens_details.cached_tokens``) so the
    nested usage shapes some SDKs return can be flattened in one call.
    """
    if usage is None:
        return 0
    if "." in key:
        head, rest = key.split(".", 1)
        sub = usage.get(head) if isinstance(usage, dict) else getattr(usage, head, None)
        return _usage_get(sub, rest)
    if isinstance(usage, dict):
        return int(usage.get(key) or 0)
    return int(getattr(usage, key, 0) or 0)


def _first_usage(usage: Any, keys: tuple[str, ...]) -> int:
    """Return the first non-zero token count among the candidate keys."""
    for key in keys:
        value = _usage_get(usage, key)
        if value:
            return value
    return 0


# Default candidate keys cover the provider-specific names across all engines.
# OpenCode passes its ``info.tokens`` sub-object, so the bare input/output/cache.*
# keys apply there; the more specific names take precedence (checked first).
#   Anthropic: input_tokens / output_tokens / cache_read_input_tokens / cache_creation_input_tokens
#   OpenAI Responses: input_tokens / output_tokens / input_tokens_details.cached_tokens
#   Google ADK: prompt_token_count / candidates_token_count / cached_content_token_count
#   OpenCode (info.tokens): input / output / cache.read / cache.write
_INPUT_KEYS = ("input_tokens", "prompt_token_count", "input")
_OUTPUT_KEYS = ("output_tokens", "candidates_token_count", "output")
_CACHE_READ_KEYS = (
    "cache_read_input_tokens",
    "cache_read_tokens",
    "input_tokens_details.cached_tokens",
    "cached_content_token_count",
    "cache.read",
)
_CACHE_CREATION_KEYS = (
    "cache_creation_input_tokens",
    "cache_creation_tokens",
    "cache.write",
)


def normalize_usage(
    usage: Any,
    *,
    input_keys: tuple[str, ...] = _INPUT_KEYS,
    output_keys: tuple[str, ...] = _OUTPUT_KEYS,
    cache_read_keys: tuple[str, ...] = _CACHE_READ_KEYS,
    cache_creation_keys: tuple[str, ...] = _CACHE_CREATION_KEYS,
) -> Usage:
    """Map an engine's provider-specific usage onto the canonical Usage (v8).

    Tolerant of dicts or objects and of nested/dotted paths. Returns a Usage with
    all four counts (0 where the engine doesn't report a kind). Built from the
    same source object the OTLP ``record_result`` consumes, so the two stay in
    sync.
    """
    from engine_core.models.events import Usage

    return Usage(
        input_tokens=_first_usage(usage, input_keys),
        output_tokens=_first_usage(usage, output_keys),
        cache_read_tokens=_first_usage(usage, cache_read_keys),
        cache_creation_tokens=_first_usage(usage, cache_creation_keys),
    )


def record_result(model: str, usage: Any, cost: Any, num_turns: Any) -> None:
    """Record token, cost, and turn metrics from a terminal ResultMessage."""
    model = model or "unknown"
    inp = _usage_get(usage, "input_tokens")
    out = _usage_get(usage, "output_tokens")
    if inp:
        _llm_tokens.add(inp, {"model": model, "kind": "input"})
    if out:
        _llm_tokens.add(out, {"model": model, "kind": "output"})
    if cost:
        _llm_cost.add(float(cost), {"model": model})
    if num_turns is not None:
        try:
            _turns.record(int(num_turns))
        except (TypeError, ValueError):
            pass


def record_job(status: str, duration_seconds: float) -> None:
    """Record a completed job's terminal status and duration."""
    _jobs.add(1, {"status": status})
    _job_duration.record(duration_seconds, {"status": status})
