import { describe, it, expect, vi, afterEach } from "vitest";

import { GET, POST } from "@/app/api/memory/[[...path]]/route";

/**
 * The memory BFF route maps /api/memory/* onto the Go backend's /v1/memory/*
 * endpoints and forwards the query string (so ?scope=&id= filters reach the
 * memory service).
 */
describe("/api/memory — BFF path mapping", () => {
  afterEach(() => vi.restoreAllMocks());

  function ctx(path?: string[]) {
    return { params: Promise.resolve({ path }) };
  }

  function mockFetch() {
    return vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json([]));
  }

  it("GET scopes → /v1/memory/scopes", async () => {
    const fetchMock = mockFetch();
    await GET(
      new Request("http://localhost:3000/api/memory/scopes"),
      ctx(["scopes"]),
    );
    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/v1\/memory\/scopes$/);
    expect(fetchMock.mock.calls[0][1]?.method).toBe("GET");
  });

  it("GET entries forwards the query string", async () => {
    const fetchMock = mockFetch();
    await GET(
      new Request("http://localhost:3000/api/memory/entries?scope=repo&id=x"),
      ctx(["entries"]),
    );
    expect(String(fetchMock.mock.calls[0][0])).toContain(
      "/v1/memory/entries?scope=repo&id=x",
    );
  });

  it("POST consolidate → /v1/memory/consolidate", async () => {
    const fetchMock = mockFetch();
    await POST(
      new Request("http://localhost:3000/api/memory/consolidate", {
        method: "POST",
        body: "{}",
      }),
      ctx(["consolidate"]),
    );
    expect(String(fetchMock.mock.calls[0][0])).toMatch(
      /\/v1\/memory\/consolidate$/,
    );
    expect(fetchMock.mock.calls[0][1]?.method).toBe("POST");
  });
});
