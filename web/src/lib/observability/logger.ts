import { trace } from "@opentelemetry/api";

/**
 * Minimal structured logger for BFF route handlers. Emits one JSON line per
 * event to stdout/stderr, stamped with the active span's trace_id/span_id so
 * logs can be correlated to traces in Grafana.
 *
 * The web tier is a thin proxy; its highest-value signal is the request trace
 * (browser -> BFF -> Go -> engine). These logs make route-level failures
 * visible instead of silent, and carry the trace id for cross-referencing.
 */
type Fields = Record<string, unknown>;

function emit(level: "info" | "warn" | "error", msg: string, fields: Fields) {
  const sc = trace.getActiveSpan()?.spanContext();
  const record: Record<string, unknown> = {
    level,
    msg,
    ...fields,
    ...(sc ? { trace_id: sc.traceId, span_id: sc.spanId } : {}),
    service: "dexiask-web",
    ts: new Date().toISOString(),
  };
  const line = JSON.stringify(record);
  if (level === "error") console.error(line);
  else console.log(line);
}

export const logger = {
  info: (msg: string, fields: Fields = {}) => emit("info", msg, fields),
  warn: (msg: string, fields: Fields = {}) => emit("warn", msg, fields),
  error: (msg: string, fields: Fields = {}) => emit("error", msg, fields),
};
