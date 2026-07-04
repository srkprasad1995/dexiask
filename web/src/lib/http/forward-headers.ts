import { context, propagation } from "@opentelemetry/api";

/**
 * Builds the headers the BFF forwards to the Dexiask Go backend.
 *
 * We forward a request id (for log correlation), inject the active W3C trace
 * context so the Go backend and engine join this request's trace, and forward
 * the session Cookie so the backend's auth middleware can resolve the user.
 */
export function buildForwardHeaders(req: Request): Record<string, string> {
  const requestId = req.headers.get("x-request-id") ?? crypto.randomUUID();
  const headers: Record<string, string> = { "X-Request-Id": requestId };

  const cookie = req.headers.get("cookie");
  if (cookie) headers["Cookie"] = cookie;

  propagation.inject(context.active(), headers);
  return headers;
}
