/**
 * Next.js instrumentation hook — runs once when the server process boots.
 *
 * Registers OpenTelemetry so API route handlers get server spans and outgoing
 * fetches to the Go backend carry W3C trace context (see forward-headers.ts).
 * Traces export over OTLP; the endpoint comes from OTEL_EXPORTER_OTLP_ENDPOINT
 * (e.g. http://localhost:4318).
 *
 * No-op when @vercel/otel finds no exporter configured, so local dev without an
 * observability stack still works.
 */
export async function register() {
  // Only the Node.js server runtime exports telemetry (not edge/browser).
  if (process.env.NEXT_RUNTIME !== "nodejs") return;

  const { registerOTel } = await import("@vercel/otel");
  registerOTel({
    serviceName: process.env.OTEL_SERVICE_NAME ?? "dexiask-web",
  });
}
