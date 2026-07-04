export const runtime = "nodejs";
export const dynamic = "force-dynamic";

/** GET /api/healthz — liveness/readiness probe target for the container. */
export function GET() {
  return Response.json({ status: "ok" });
}
