import { NextResponse, type NextRequest } from "next/server";

import { SESSION_COOKIE } from "@/lib/auth/constants";
import { env } from "@/lib/env";

/**
 * When auth is enabled, gate app pages behind a session cookie: a request with
 * no session is redirected to /login. In dev-fallback mode (AUTH_ENABLED unset)
 * this is a no-op so the zero-config experience is preserved.
 *
 * Only page navigations are gated — /api/* (incl. the OAuth flow), /login, and
 * Next internals are always allowed through.
 */
export function middleware(req: NextRequest) {
  if (!env.authEnabled) return NextResponse.next();

  const { pathname } = req.nextUrl;
  if (
    pathname.startsWith("/api/") ||
    pathname.startsWith("/_next/") ||
    pathname === "/login" ||
    pathname === "/favicon.ico"
  ) {
    return NextResponse.next();
  }

  if (req.cookies.get(SESSION_COOKIE)) return NextResponse.next();

  const url = req.nextUrl.clone();
  url.pathname = "/login";
  return NextResponse.redirect(url);
}

export const config = {
  matcher: ["/((?!_next/static|_next/image).*)"],
};
