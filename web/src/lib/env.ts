/**
 * Server-side environment access. The browser never reads these — only BFF
 * route handlers and server components do.
 */
export const env = {
  /** Base URL of the Dexiask Go backend (REST gateway). */
  goBackendUrl: process.env.GO_BACKEND_URL ?? "http://localhost:8080",
};
