/**
 * Server-side environment access. The browser never reads these — only BFF
 * route handlers and server components do.
 */
export const env = {
  /** Base URL of the Dexiask Go backend (REST gateway). */
  goBackendUrl: process.env.GO_BACKEND_URL ?? "http://localhost:8080",
  /**
   * Whether GitHub-OAuth login is enforced. Mirrors the backend's RequireAuth:
   * when false the app runs in dev-fallback mode (no login gate). Set
   * AUTH_ENABLED=true in the web service env when a GitHub OAuth app is wired.
   */
  authEnabled: process.env.AUTH_ENABLED === "true",
};
