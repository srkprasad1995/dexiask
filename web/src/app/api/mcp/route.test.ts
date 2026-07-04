import { describe, it, expect, vi, afterEach } from "vitest";

import { DELETE, GET, POST, PUT } from "@/app/api/mcp/[[...path]]/route";

/**
 * The MCP BFF route is a thin catch-all that maps /api/mcp/* onto the Go
 * backend's /v1/mcp-servers/* CRUD endpoints. These tests pin the path mapping
 * per HTTP method so the browser never has to know the backend route shape.
 */
describe("/api/mcp — BFF path mapping", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  function ctx(path?: string[]) {
    return { params: Promise.resolve({ path }) };
  }

  function mockFetch() {
    return vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json({ mcpServers: [] }));
  }

  it("GET (no path) → /v1/mcp-servers", async () => {
    const fetchMock = mockFetch();
    await GET(new Request("http://localhost:3000/api/mcp"), ctx());
    expect(String(fetchMock.mock.calls[0][0])).toContain("/v1/mcp-servers");
    expect(String(fetchMock.mock.calls[0][0])).not.toContain("/v1/mcp-servers/");
    expect(fetchMock.mock.calls[0][1]?.method).toBe("GET");
  });

  it("POST (no path) → /v1/mcp-servers", async () => {
    const fetchMock = mockFetch();
    await POST(
      new Request("http://localhost:3000/api/mcp", {
        method: "POST",
        body: JSON.stringify({ name: "gh", type: "http", url: "http://x" }),
      }),
      ctx(),
    );
    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/v1\/mcp-servers$/);
    expect(fetchMock.mock.calls[0][1]?.method).toBe("POST");
  });

  it("PUT {id} → /v1/mcp-servers/{id}", async () => {
    const fetchMock = mockFetch();
    await PUT(
      new Request("http://localhost:3000/api/mcp/m1", {
        method: "PUT",
        body: JSON.stringify({ enabled: false }),
      }),
      ctx(["m1"]),
    );
    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/v1\/mcp-servers\/m1$/);
    expect(fetchMock.mock.calls[0][1]?.method).toBe("PUT");
  });

  it("DELETE {id} → /v1/mcp-servers/{id}", async () => {
    const fetchMock = mockFetch();
    await DELETE(
      new Request("http://localhost:3000/api/mcp/m1", { method: "DELETE" }),
      ctx(["m1"]),
    );
    expect(String(fetchMock.mock.calls[0][0])).toMatch(/\/v1\/mcp-servers\/m1$/);
    expect(fetchMock.mock.calls[0][1]?.method).toBe("DELETE");
  });
});
