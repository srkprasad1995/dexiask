"""
OpenTelemetry initialization (best-effort).

Exports traces over OTLP/HTTP to the platform collector and instruments the
FastAPI app. A no-op when OTel is disabled or the SDK isn't importable, so the
service never fails to start over telemetry.
"""
from __future__ import annotations

import logging
import os

log = logging.getLogger("indexer.observability")


def init_observability(app, service_name: str = "dexiask-indexer") -> None:
    if os.getenv("OTEL_SDK_DISABLED", "").lower() == "true":
        return
    try:
        from opentelemetry import trace
        from opentelemetry.exporter.otlp.proto.http.trace_exporter import OTLPSpanExporter
        from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
        from opentelemetry.sdk.resources import Resource
        from opentelemetry.sdk.trace import TracerProvider
        from opentelemetry.sdk.trace.export import BatchSpanProcessor

        provider = TracerProvider(resource=Resource.create({"service.name": service_name}))
        provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter()))
        trace.set_tracer_provider(provider)
        FastAPIInstrumentor.instrument_app(app)
        log.info("OpenTelemetry initialized for %s", service_name)
    except Exception as e:  # pragma: no cover - depends on env
        log.warning("OpenTelemetry init skipped: %s", e)
