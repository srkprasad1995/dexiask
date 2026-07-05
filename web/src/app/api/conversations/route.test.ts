import { describe, it, expect, vi, afterEach } from "vitest";

import { GET } from "@/app/api/conversations/[[...path]]/route";

/** The conversations BFF maps /api/conversations/* onto /v1/conversations/*. */
describe("/api/conversations — BFF path mapping", () => {
  afterEach(() => vi.restoreAllMocks());

  function ctx(path?: string[]) {
    return { params: Promise.resolve({ path }) };
  }
  function mockFetch() {
    return vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json({ conversations: [] }));
  }

  it("list → /v1/conversations, forwarding the query", async () => {
    const f = mockFetch();
    await GET(
      new Request("http://localhost:3000/api/conversations?page_size=100"),
      ctx(),
    );
    expect(String(f.mock.calls[0][0])).toContain(
      "/v1/conversations?page_size=100",
    );
  });

  it("transcript → /v1/conversations/{id}/messages", async () => {
    const f = mockFetch();
    await GET(
      new Request("http://localhost:3000/api/conversations/c1/messages"),
      ctx(["c1", "messages"]),
    );
    expect(String(f.mock.calls[0][0])).toMatch(
      /\/v1\/conversations\/c1\/messages$/,
    );
  });
});
