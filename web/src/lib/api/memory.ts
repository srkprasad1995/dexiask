/**
 * TanStack Query hooks for the memory service, via the same-origin BFF proxy
 * under /api/memory/*, which the Go backend reverse-proxies to the memory
 * service (/v1/memory/...) after injecting the authenticated user id.
 */

"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { apiGet, apiSend } from "@/lib/api/fetcher";

export interface MemoryScope {
  scope: string;
  id: string;
  label: string;
}

export interface MemoryEntry {
  topic: string;
  entry: string;
  scope: string;
  scope_id: string;
  preview: string;
}

export interface WorkingFile {
  filename: string;
  date: string;
  entry_count: number;
  preview: string;
}

export const memoryKeys = {
  scopes: () => ["memory", "scopes"] as const,
  entries: (scope: string, id: string) =>
    ["memory", "entries", scope, id] as const,
  working: (scope: string, id: string) =>
    ["memory", "working", scope, id] as const,
};

/** All scopes (global/user/repo) that hold consolidated or working memory. */
export function useMemoryScopes() {
  return useQuery({
    queryKey: memoryKeys.scopes(),
    queryFn: () => apiGet<MemoryScope[]>("/api/memory/scopes"),
    staleTime: 10_000,
    retry: false,
  });
}

/** Consolidated entries for a scope. */
export function useMemoryEntries(scope: string | undefined, id: string) {
  return useQuery({
    queryKey: memoryKeys.entries(scope ?? "", id),
    queryFn: () =>
      apiGet<MemoryEntry[]>(
        `/api/memory/entries?scope=${encodeURIComponent(scope ?? "")}&id=${encodeURIComponent(id)}`,
      ),
    enabled: !!scope,
    retry: false,
  });
}

/** Raw working-memory observation files for a scope. */
export function useWorkingMemory(scope: string | undefined, id: string) {
  return useQuery({
    queryKey: memoryKeys.working(scope ?? "", id),
    queryFn: () =>
      apiGet<WorkingFile[]>(
        `/api/memory/working?scope=${encodeURIComponent(scope ?? "")}&id=${encodeURIComponent(id)}`,
      ),
    enabled: !!scope,
    retry: false,
  });
}

/** Trigger a manual dream consolidation now. */
export function useConsolidate() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => apiSend("/api/memory/consolidate", "POST", {}),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["memory"] });
    },
  });
}
