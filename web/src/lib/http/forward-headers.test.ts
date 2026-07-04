import { describe, it, expect } from "vitest";

import { buildForwardHeaders } from "@/lib/http/forward-headers";

/**
 * buildForwardHeaders forwards the session Cookie to the backend so its auth
 * middleware can resolve the user. We use a minimal fake request because the
 * Fetch spec forbids setting a Cookie header via the Request constructor.
 */
function fakeReq(headers: Record<string, string>): Request {
  return {
    headers: {
      get: (k: string) => headers[k.toLowerCase()] ?? null,
    },
  } as unknown as Request;
}

describe("buildForwardHeaders", () => {
  it("forwards the Cookie header when present", () => {
    const out = buildForwardHeaders(fakeReq({ cookie: "dexiask_session=abc" }));
    expect(out["Cookie"]).toBe("dexiask_session=abc");
    expect(out["X-Request-Id"]).toBeTruthy();
  });

  it("omits Cookie when the request has none", () => {
    const out = buildForwardHeaders(fakeReq({}));
    expect(out["Cookie"]).toBeUndefined();
  });
});
