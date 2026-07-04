import { NextResponse } from "next/server";

import { env } from "@/lib/env";
import { buildForwardHeaders } from "@/lib/http/forward-headers";

/**
 * Proxies a request to the Go backend, forwarding trace/request headers and
 * relaying the status + JSON body. Used by the thin catch-all BFF routes.
 */
export async function proxy(
  req: Request,
  method: string,
  path: string,
): Promise<NextResponse> {
  const init: RequestInit = {
    method,
    headers: {
      ...buildForwardHeaders(req),
      "Content-Type": "application/json",
    },
  };
  if (method !== "GET" && method !== "DELETE") {
    init.body = await req.text();
  }
  const res = await fetch(`${env.goBackendUrl}${path}`, init);
  if (res.status === 204) return new NextResponse(null, { status: 204 });
  const data = await res.json().catch(() => ({}));
  return NextResponse.json(data, { status: res.status });
}
