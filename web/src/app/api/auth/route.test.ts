// @vitest-environment node
import { describe, it, expect, vi, afterEach } from "vitest";

import { GET, POST } from "@/app/api/auth/[[...path]]/route";

/**
 * The auth BFF route maps /api/auth/* onto the backend /v1/auth/* endpoints and,
 * critically, relays redirects and Set-Cookie headers so the session cookie lands
 * on the web origin the browser talks to. Runs under the node environment so
 * Set-Cookie handling matches the real server runtime.
 */
describe("/api/auth — OAuth BFF proxy", () => {
  afterEach(() => vi.restoreAllMocks());

  function ctx(path?: string[]) {
    return { params: Promise.resolve({ path }) };
  }

  function mockResponse(status: number, setCookie: string, location?: string) {
    const headers = new Headers();
    headers.append("set-cookie", setCookie);
    if (location) headers.set("location", location);
    return new Response(null, { status, headers });
  }

  it("login → maps to /v1/auth/login and relays the redirect + cookie", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(
        mockResponse(
          302,
          "dexiask_oauth_state=x; Path=/; HttpOnly",
          "https://github.com/login/oauth/authorize?state=x",
        ),
      );

    const res = await GET(
      new Request("http://localhost:3000/api/auth/login"),
      ctx(["login"]),
    );

    expect(String(fetchMock.mock.calls[0][0])).toContain("/v1/auth/login");
    expect(fetchMock.mock.calls[0][1]?.redirect).toBe("manual");
    expect(res.status).toBe(302);
    expect(res.headers.get("location")).toContain(
      "github.com/login/oauth/authorize",
    );
    expect(res.headers.get("set-cookie")).toContain("dexiask_oauth_state=x");
  });

  it("callback → relays the session Set-Cookie and app redirect", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      mockResponse(
        302,
        "dexiask_session=sess.sig; Path=/; HttpOnly",
        "http://localhost:3000/",
      ),
    );

    const res = await GET(
      new Request("http://localhost:3000/api/auth/callback?code=abc&state=x"),
      ctx(["callback"]),
    );

    expect(res.status).toBe(302);
    expect(res.headers.get("set-cookie")).toContain("dexiask_session=");
  });

  it("logout → 204 relays the cookie-clearing Set-Cookie", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      mockResponse(204, "dexiask_session=; Path=/; Max-Age=0"),
    );

    const res = await POST(
      new Request("http://localhost:3000/api/auth/logout", { method: "POST" }),
      ctx(["logout"]),
    );

    expect(res.status).toBe(204);
    expect(res.headers.get("set-cookie")).toContain("Max-Age=0");
  });
});
