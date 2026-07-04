/**
 * TanStack Query hooks for the Dexiask code indexer. These hit the same-origin
 * BFF proxy under /api/indexer/*, which the Go backend reverse-proxies to the
 * indexer service (`/v1/indexer/...`).
 *
 * Endpoints (proxied verbatim through the Go backend to the indexer service):
 *   GET  /v1/indexer/v1/repos    — list registered repos + their index status
 *   POST /v1/indexer/v1/repos    — register a repo (git url or /workspace path)
 *   POST /v1/indexer/reindex     — (re)index one repo or all
 *   GET  /v1/indexer/v1/status   — overall indexer health / progress
 *   POST /v1/indexer/v1/search   — semantic code search
 */

"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { apiGet, apiSend } from "@/lib/api/fetcher";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface Repo {
  id: string;
  /** A git remote (https/ssh) OR omitted for a local /workspace path. */
  url?: string;
  /** A path under the indexer's /workspace mount (alternative to `url`). */
  path?: string;
  indexed: boolean;
  /** Free-form status string, e.g. "idle" | "indexing" | "error". */
  status?: string;
  /** Number of embedded chunks, when reported. */
  chunks?: number;
  /** branch -> commit sha, when reported. */
  branches?: Record<string, string>;
}

export interface ReposResponse {
  repos: Repo[];
}

export interface IndexerStatus {
  /** Overall health/progress string, e.g. "ready" | "indexing". */
  status?: string;
  repos?: Repo[];
}

export interface AddRepoInput {
  id: string;
  /** A git remote cloned + fetched directly (public needs no auth). */
  url?: string;
  /** A local checkout under the indexer's /workspace root. */
  path?: string;
}

export interface ReindexInput {
  /** Repo id to reindex; omit to reindex everything. */
  repo?: string;
  /** Force a full rebuild rather than an incremental update. */
  full?: boolean;
}

export interface SearchResult {
  repo?: string;
  /** "code" (default) or "doc" for a generated domain-knowledge doc. */
  contentType?: string;
  path: string;
  /** Title of the domain doc (set when contentType === "doc"). */
  title?: string;
  score?: number;
  /** The matching code snippet / chunk text. */
  content?: string;
  startLine?: number;
  endLine?: number;
  language?: string;
}

export interface SearchResponse {
  results: SearchResult[];
}

/** A generated domain-knowledge doc for the "Docs" tab. */
export interface DomainDoc {
  title: string;
  category: string;
  slug: string;
  body: string;
}

export interface DomainDocsResponse {
  docs: DomainDoc[];
}

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const indexerKeys = {
  repos: () => ["indexer", "repos"] as const,
  status: () => ["indexer", "status"] as const,
  gitToken: () => ["indexer", "git-token"] as const,
  domainDocs: (repo: string) => ["indexer", "docs", repo] as const,
};

/** Whether a git access token is configured server-side (value never leaves the indexer). */
export interface GitTokenStatus {
  configured: boolean;
}

// ---------------------------------------------------------------------------
// Hooks
// ---------------------------------------------------------------------------

/** The registered repos and their per-repo index status. */
export function useRepos() {
  return useQuery({
    queryKey: indexerKeys.repos(),
    queryFn: () => apiGet<ReposResponse>("/api/indexer/v1/repos"),
    staleTime: 10_000,
  });
}

/** Overall indexer health / progress. */
export function useIndexerStatus() {
  return useQuery({
    queryKey: indexerKeys.status(),
    queryFn: () => apiGet<IndexerStatus>("/api/indexer/v1/status"),
    refetchInterval: 10_000,
    retry: false,
  });
}

/** Register (or replace) a tracked repo. */
export function useAddRepo() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: AddRepoInput) =>
      apiSend<Repo>("/api/indexer/v1/repos", "POST", input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: indexerKeys.repos() });
      qc.invalidateQueries({ queryKey: indexerKeys.status() });
    },
  });
}

/** Trigger a reindex for one repo (or all when `repo` is omitted). */
export function useReindex() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: ReindexInput) =>
      apiSend<IndexerStatus>("/api/indexer/reindex", "POST", input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: indexerKeys.repos() });
      qc.invalidateQueries({ queryKey: indexerKeys.status() });
    },
  });
}

/** Run a semantic code search across the index. */
export function useSearch() {
  return useMutation({
    mutationFn: (vars: { query: string; repo?: string; limit?: number }) =>
      apiSend<SearchResponse>("/api/indexer/v1/search", "POST", vars),
  });
}

/** Generated domain-knowledge docs for a repo (empty until enabled + indexed). */
export function useDomainDocs(repo: string | undefined) {
  return useQuery({
    queryKey: indexerKeys.domainDocs(repo ?? ""),
    queryFn: () =>
      apiGet<DomainDocsResponse>(
        `/api/indexer/v1/docs/${encodeURIComponent(repo ?? "")}`,
      ),
    enabled: !!repo,
    staleTime: 30_000,
    retry: false,
  });
}

/** Whether a git access token is configured (used to clone private repos). */
export function useGitTokenStatus() {
  return useQuery({
    queryKey: indexerKeys.gitToken(),
    queryFn: () => apiGet<GitTokenStatus>("/api/indexer/v1/git-token"),
    staleTime: 10_000,
    retry: false,
  });
}

/** Set or clear the git access token (an empty token clears it). */
export function useSetGitToken() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (token: string) =>
      apiSend<GitTokenStatus>("/api/indexer/v1/git-token", "PUT", { token }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: indexerKeys.gitToken() });
    },
  });
}
