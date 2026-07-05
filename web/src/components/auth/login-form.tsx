"use client";

import { useEffect, useState, type FormEvent } from "react";
import { Loader2 } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

type AuthConfig = { authEnabled: boolean; oauth: boolean; tokenLogin: boolean };

/**
 * Login form. Primary method is **GitHub token login**: paste a personal access
 * token (with `repo` + `read:user` scopes) — the backend validates it against the
 * GitHub API, resolves your identity, and stores it for private-repo cloning +
 * repo-access checks. If a GitHub OAuth app is configured, a "Sign in with
 * GitHub" button is also offered.
 */
export function LoginForm() {
  const [config, setConfig] = useState<AuthConfig | null>(null);
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    fetch("/api/auth/config")
      .then((r) => (r.ok ? (r.json() as Promise<AuthConfig>) : null))
      .then(setConfig)
      .catch(() => setConfig(null));
  }, []);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    const t = token.trim();
    if (!t) return;
    setBusy(true);
    setError("");
    try {
      const res = await fetch("/api/auth/token-login", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ token: t }),
      });
      if (!res.ok) {
        const body = (await res.json().catch(() => ({}))) as { error?: string };
        setError(body.error ?? `Login failed (${res.status})`);
        setBusy(false);
        return;
      }
      window.location.href = "/";
    } catch {
      setError("Login failed — is the backend reachable?");
      setBusy(false);
    }
  }

  return (
    <div className="w-full max-w-sm space-y-5 rounded-xl border border-black/10 p-8 shadow-sm dark:border-white/10">
      <div className="text-center">
        <h1 className="font-plex-serif text-2xl font-semibold">Dexiask</h1>
        <p className="mt-2 text-sm text-black/60 dark:text-white/60">
          Sign in with a GitHub token to continue.
        </p>
      </div>

      <form onSubmit={onSubmit} className="space-y-3">
        <Input
          type="password"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          placeholder="GitHub token (ghp_… / github_pat_…)"
          autoComplete="off"
        />
        <Button type="submit" disabled={busy} className="w-full gap-1.5">
          {busy && <Loader2 className="h-4 w-4 animate-spin" />}
          Sign in with token
        </Button>
        {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
        <p className="text-xs text-black/50 dark:text-white/50">
          Needs scopes <code>repo</code> (to index/read your repos) and{" "}
          <code>read:user</code>. Stored encrypted; never shown again.
        </p>
      </form>

      {config?.oauth && (
        <>
          <div className="flex items-center gap-3 text-xs text-black/40 dark:text-white/40">
            <span className="h-px flex-1 bg-black/10 dark:bg-white/10" />
            or
            <span className="h-px flex-1 bg-black/10 dark:bg-white/10" />
          </div>
          {/* Full-page navigation to a route handler that 302s to GitHub. */}
          {/* eslint-disable-next-line @next/next/no-html-link-for-pages */}
          <a
            href="/api/auth/login"
            className="inline-flex w-full items-center justify-center rounded-lg border border-black/15 px-4 py-2.5 text-sm font-medium transition hover:bg-black/5 dark:border-white/15 dark:hover:bg-white/5"
          >
            Sign in with GitHub
          </a>
        </>
      )}
    </div>
  );
}
