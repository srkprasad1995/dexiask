"use client";

import { useState, type FormEvent } from "react";
import { Loader2, Trash2, UserPlus } from "lucide-react";
import { toast } from "sonner";

import {
  useCreateInvite,
  useDeleteInvite,
  useInvites,
  useUsers,
} from "@/lib/api/admin";
import { useIsAdmin, useUser } from "@/lib/auth/use-user";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";

export function AdminView() {
  const { loading } = useUser();
  const isAdmin = useIsAdmin();
  const users = useUsers();
  const invites = useInvites();
  const createInvite = useCreateInvite();
  const deleteInvite = useDeleteInvite();
  const [login, setLogin] = useState("");

  if (loading) {
    return (
      <div className="mx-auto w-full max-w-3xl p-8">
        <Skeleton className="h-8 w-full" />
      </div>
    );
  }
  if (!isAdmin) {
    return (
      <div className="mx-auto w-full max-w-3xl p-8">
        <p className="text-sm text-muted-foreground">
          Admins only. Ask a workspace admin if you need access.
        </p>
      </div>
    );
  }

  function invite(e: FormEvent) {
    e.preventDefault();
    const l = login.trim();
    if (!l) return;
    createInvite.mutate(l, {
      onSuccess: () => {
        toast.success(`Invited ${l}`);
        setLogin("");
      },
      onError: (err) => toast.error(err.message || "Failed to invite"),
    });
  }

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto w-full max-w-3xl space-y-8 px-4 py-8">
        <header className="space-y-1">
          <h1 className="text-lg font-semibold tracking-tight">Team</h1>
          <p className="text-sm text-muted-foreground">
            Invite people by GitHub username. They sign in with their own GitHub
            token and join as members; admins manage indexing, MCP servers, and
            memory.
          </p>
        </header>

        <section className="space-y-3">
          <h2 className="text-sm font-semibold">Invite a member</h2>
          <form onSubmit={invite} className="flex gap-2">
            <Input
              value={login}
              onChange={(e) => setLogin(e.target.value)}
              placeholder="GitHub username (e.g. octocat)"
            />
            <Button type="submit" disabled={createInvite.isPending} className="gap-1.5">
              {createInvite.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <UserPlus className="h-4 w-4" />
              )}
              Invite
            </Button>
          </form>
        </section>

        <section className="space-y-3">
          <h2 className="text-sm font-semibold">Pending invites</h2>
          {(invites.data ?? []).length === 0 && (
            <p className="text-sm text-muted-foreground">No pending invites.</p>
          )}
          <div className="space-y-1">
            {(invites.data ?? []).map((inv) => (
              <div
                key={inv.login}
                className="flex items-center gap-2 rounded-lg border bg-card px-3 py-2 text-sm"
              >
                <span className="min-w-0 flex-1 truncate font-plex-mono">
                  {inv.login}
                </span>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={`Revoke invite for ${inv.login}`}
                  onClick={() =>
                    deleteInvite.mutate(inv.login, {
                      onError: (err) => toast.error(err.message || "Failed"),
                    })
                  }
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            ))}
          </div>
        </section>

        <section className="space-y-3">
          <h2 className="text-sm font-semibold">Members</h2>
          <div className="space-y-1">
            {(users.data ?? []).map((u) => (
              <div
                key={u.id}
                className="flex items-center gap-2 rounded-lg border bg-card px-3 py-2 text-sm"
              >
                <span className="min-w-0 flex-1 truncate">{u.login}</span>
                <Badge variant={u.role === "admin" ? "build" : "secondary"}>
                  {u.role}
                </Badge>
              </div>
            ))}
          </div>
        </section>
      </div>
    </div>
  );
}
