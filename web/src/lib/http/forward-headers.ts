import { context, propagation } from "@opentelemetry/api";

/**
 * Builds the headers the BFF forwards to the Dexiask Go backend.
 *
 * Dexiask is a single-tenant, no-auth deployment: there is no Authorization or
 * X-Workspace-Id. We forward only a request id (for log correlation) and inject
 * the active W3C trace context so the Go backend and engine join this request's
 * trace (one end-to-end trace per chat turn).
 */
export function buildForwardHeaders(req: Request): Record<string, string> {
  const requestId = req.headers.get("x-request-id") ?? crypto.randomUUID();
  const headers: Record<string, string> = { "X-Request-Id": requestId };

  propagation.inject(context.active(), headers);
  return headers;
}
