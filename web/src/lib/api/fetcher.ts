"use client";

/**
 * Small client fetch helpers used by the browser to talk to the same-origin BFF
 * routes. Dexiask has no auth, so these are thin JSON wrappers that surface a
 * typed error on non-2xx responses.
 */

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

/** GET JSON. */
export async function apiGet<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) {
    const body = (await res.json().catch(() => ({}))) as { error?: string };
    throw new ApiError(
      res.status,
      body.error ?? `request failed: ${res.status}`,
    );
  }
  return res.json();
}

/** Send a JSON body (POST/PATCH/DELETE). */
export async function apiSend<T>(
  url: string,
  method: string,
  body?: unknown,
): Promise<T> {
  const res = await fetch(url, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    const errBody = (await res.json().catch(() => ({}))) as { error?: string };
    throw new ApiError(
      res.status,
      errBody.error ?? `request failed: ${res.status}`,
    );
  }
  return res.status === 204 ? (undefined as T) : res.json();
}
