/**
 * Admin-only hooks for the workspace roster + invites, via the BFF proxy under
 * /api/admin/*. The Go backend enforces the admin role (403 otherwise).
 */

"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { apiGet, apiSend } from "@/lib/api/fetcher";

export interface WorkspaceUser {
  id: string;
  login: string;
  name: string;
  avatar_url?: string;
  role: string;
}

export interface Invite {
  login: string;
  created_by: string;
  created_at: string;
}

export const adminKeys = {
  users: () => ["admin", "users"] as const,
  invites: () => ["admin", "invites"] as const,
};

export function useUsers() {
  return useQuery({
    queryKey: adminKeys.users(),
    queryFn: () =>
      apiGet<{ users: WorkspaceUser[] }>("/api/admin/users").then((r) => r.users),
    retry: false,
  });
}

export function useInvites() {
  return useQuery({
    queryKey: adminKeys.invites(),
    queryFn: () =>
      apiGet<{ invites: Invite[] }>("/api/admin/invites").then((r) => r.invites),
    retry: false,
  });
}

export function useCreateInvite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (login: string) =>
      apiSend<Invite>("/api/admin/invites", "POST", { login }),
    onSuccess: () => qc.invalidateQueries({ queryKey: adminKeys.invites() }),
  });
}

export function useDeleteInvite() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (login: string) =>
      apiSend<void>(`/api/admin/invites/${encodeURIComponent(login)}`, "DELETE"),
    onSuccess: () => qc.invalidateQueries({ queryKey: adminKeys.invites() }),
  });
}
