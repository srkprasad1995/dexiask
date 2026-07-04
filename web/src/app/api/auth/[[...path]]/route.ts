import { NextResponse } from "next/server";

import { env } from "@/lib/env";
import { buildForwardHeaders } from "@/lib/http/forward-headers";

export const runtime = "nodejs";
export const dynamic = "force-dynamic";

/**
 * Catch-all proxy for the GitHub OAuth flow. It maps /api/auth/* onto the Go
 * backend's /v1/auth/* endpoints and — unlike the JSON BFF proxy — relays both
 * redirects (login → GitHub, callback → app) and Set-Cookie headers, so the
 * server session cookie is stored on the web origin the browser talks to.
 *   GET  /api/auth/login    -> GET  /v1/auth/login    (302 to GitHub)
 *   GET  /api/auth/callback -> GET  /v1/auth/callback (302 to app + Set-Cookie)
 *   POST /api/auth/logout   -> POST /v1/auth/logout   (clears cookie)
 *   GET  /api/auth/me       -> GET  /v1/auth/me        (JSON)
 */
type Ctx = { params: Promise<{ path?: string[] }> };

async function handle(
  req: Request,
  method: string,
  { params }: Ctx,
): Promise<NextResponse> {
  const path = (await params).path;
  const suffix = path && path.length ? "/" + path.join("/") : "";
  const { search } = new URL(req.url);
  const url = `${env.goBackendUrl}/v1/auth${suffix}${search}`;

  const init: RequestInit = {
    method,
    headers: buildForwardHeaders(req),
    // Handle the backend's 302 ourselves so we can rewrite it into a same-origin
    // redirect and relay the Set-Cookie header.
    redirect: "manual",
  };
  if (method !== "GET" && method !== "DELETE") {
    init.body = await req.text();
  }

  const res = await fetch(url, init);
  const setCookies = res.headers.getSetCookie();
  const location = res.headers.get("location");

  let out: NextResponse;
  if (location && res.status >= 300 && res.status < 400) {
    out = NextResponse.redirect(location, { status: 302 });
  } else if (res.status === 204) {
    out = new NextResponse(null, { status: 204 });
  } else {
    const data = await res.json().catch(() => ({}));
    out = NextResponse.json(data, { status: res.status });
  }
  for (const cookie of setCookies) out.headers.append("set-cookie", cookie);
  return out;
}

export async function GET(req: Request, ctx: Ctx) {
  return handle(req, "GET", ctx);
}
export async function POST(req: Request, ctx: Ctx) {
  return handle(req, "POST", ctx);
}
