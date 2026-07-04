"use client";

import { useEffect, useState } from "react";

/** The current user as returned by GET /api/auth/me. */
export type Me = {
  id: string;
  login: string;
  name: string;
  avatar_url?: string;
};

type UserState = {
  user: Me | null;
  loading: boolean;
};

/**
 * useUser fetches the current user from the BFF /api/auth/me endpoint. In
 * dev-fallback mode it resolves the local dev user; when unauthenticated (auth
 * enabled, no session) it resolves null.
 */
export function useUser(): UserState {
  const [state, setState] = useState<UserState>({ user: null, loading: true });

  useEffect(() => {
    let active = true;
    fetch("/api/auth/me")
      .then((res) => (res.ok ? (res.json() as Promise<Me>) : null))
      .then((user) => {
        if (active) setState({ user, loading: false });
      })
      .catch(() => {
        if (active) setState({ user: null, loading: false });
      });
    return () => {
      active = false;
    };
  }, []);

  return state;
}
