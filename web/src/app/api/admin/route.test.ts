import { describe, it, expect, vi, afterEach } from "vitest";

import { DELETE, GET, POST } from "@/app/api/admin/[[...path]]/route";

/** The admin BFF maps /api/admin/* onto the backend's /v1/* admin routes. */
describe("/api/admin — BFF path mapping", () => {
  afterEach(() => vi.restoreAllMocks());

  function ctx(path?: string[]) {
    return { params: Promise.resolve({ path }) };
  }
  function mockFetch() {
    return vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json({}));
  }

  it("GET users → /v1/users", async () => {
    const f = mockFetch();
    await GET(new Request("http://localhost:3000/api/admin/users"), ctx(["users"]));
    expect(String(f.mock.calls[0][0])).toMatch(/\/v1\/users$/);
  });

  it("POST invites → /v1/invites", async () => {
    const f = mockFetch();
    await POST(
      new Request("http://localhost:3000/api/admin/invites", {
        method: "POST",
        body: JSON.stringify({ login: "octocat" }),
      }),
      ctx(["invites"]),
    );
    expect(String(f.mock.calls[0][0])).toMatch(/\/v1\/invites$/);
    expect(f.mock.calls[0][1]?.method).toBe("POST");
  });

  it("DELETE invites/{login} → /v1/invites/{login}", async () => {
    const f = mockFetch();
    await DELETE(
      new Request("http://localhost:3000/api/admin/invites/octocat", {
        method: "DELETE",
      }),
      ctx(["invites", "octocat"]),
    );
    expect(String(f.mock.calls[0][0])).toMatch(/\/v1\/invites\/octocat$/);
  });
});
