import { describe, it, expect, vi, afterEach } from "vitest";

import { POST } from "@/app/api/upload/route";

/**
 * The upload route proxies the multipart form to the Go backend and then
 * rewrites the Go-issued `url` to a same-origin BFF path (/api/upload/<id>) so
 * the browser always fetches attachments from this app.
 */
describe("POST /api/upload — URL rewrite", () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  function makeRequest(): Request {
    const fd = new FormData();
    fd.append("file", new Blob(["hello"], { type: "text/plain" }), "note.txt");
    fd.append("uploadBucket", "bucket-1");
    return new Request("http://localhost:3000/api/upload", {
      method: "POST",
      body: fd,
    });
  }

  it("rewrites the backend url to /api/upload/<id>", async () => {
    const backendPayload = {
      id: "abc123",
      url: "http://backend:8080/v1/attachments/abc123",
      mediaType: "text/plain",
      filename: "note.txt",
      size: 5,
    };
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json(backendPayload));

    const res = await POST(makeRequest());
    const body = await res.json();

    // The backend was called once for the attachment upload.
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const calledUrl = fetchMock.mock.calls[0][0];
    expect(String(calledUrl)).toContain("/v1/attachments");

    // The returned url is rewritten to the same-origin BFF path; other fields
    // are preserved verbatim.
    expect(body.url).toBe("/api/upload/abc123");
    expect(body.id).toBe("abc123");
    expect(body.mediaType).toBe("text/plain");
    expect(body.filename).toBe("note.txt");
    expect(body.size).toBe(5);
  });

  it("relays a backend error status", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response("nope", { status: 413 }),
    );

    const res = await POST(makeRequest());
    expect(res.status).toBe(413);
    const body = await res.json();
    expect(body.error).toBeDefined();
  });
});
