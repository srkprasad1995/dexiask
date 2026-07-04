"use client";

import { LogOut } from "lucide-react";

import { Button } from "@/components/ui/button";
import { useUser } from "@/lib/auth/use-user";

/**
 * Compact account indicator in the app header: shows the signed-in GitHub user
 * with a logout button. Renders nothing while loading or when no user resolves
 * (e.g. auth disabled and /me withheld).
 */
export function UserBadge() {
  const { user, loading } = useUser();
  if (loading || !user) return null;

  async function logout() {
    await fetch("/api/auth/logout", { method: "POST" });
    window.location.href = "/login";
  }

  return (
    <div className="flex items-center gap-2">
      {user.avatar_url ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={user.avatar_url}
          alt=""
          className="h-6 w-6 rounded-full"
          width={24}
          height={24}
        />
      ) : null}
      <span className="hidden text-sm text-muted-foreground sm:inline">
        {user.login}
      </span>
      <Button
        variant="ghost"
        size="icon"
        aria-label="Sign out"
        onClick={logout}
      >
        <LogOut className="h-4 w-4" />
      </Button>
    </div>
  );
}
