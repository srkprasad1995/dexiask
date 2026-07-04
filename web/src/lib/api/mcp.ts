/**
 * TanStack Query hooks for the user-defined MCP servers. These hit the
 * same-origin BFF proxy under /api/mcp/*, which the Go backend serves from its
 * CRUD store (`/v1/mcp-servers/...`). Every enabled server is injected into
 * each ask turn alongside the built-in indexer server.
 */

"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { apiGet, apiSend } from "@/lib/api/fetcher";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type McpServerType = "http" | "sse";

export interface McpServer {
  id: string;
  name: string;
  type: McpServerType;
  url: string;
  /** Optional request headers (e.g. auth). Values are stored server-side. */
  headers?: Record<string, string>;
  enabled: boolean;
}

export interface McpServersResponse {
  mcpServers: McpServer[];
}

export interface AddMcpServerInput {
  name: string;
  type: McpServerType;
  url: string;
  headers?: Record<string, string>;
  enabled?: boolean;
}

export interface UpdateMcpServerInput {
  id: string;
  name?: string;
  type?: McpServerType;
  url?: string;
  headers?: Record<string, string>;
  enabled?: boolean;
}

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const mcpKeys = {
  servers: () => ["mcp", "servers"] as const,
};

// ---------------------------------------------------------------------------
// Hooks
// ---------------------------------------------------------------------------

/** The registered custom MCP servers. */
export function useMcpServers() {
  return useQuery({
    queryKey: mcpKeys.servers(),
    queryFn: () => apiGet<McpServersResponse>("/api/mcp"),
    staleTime: 10_000,
  });
}

/** Register a new MCP server. */
export function useAddMcpServer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: AddMcpServerInput) =>
      apiSend<McpServer>("/api/mcp", "POST", input),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: mcpKeys.servers() });
    },
  });
}

/** Update an MCP server (edits or the enabled toggle). */
export function useUpdateMcpServer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...patch }: UpdateMcpServerInput) =>
      apiSend<McpServer>(`/api/mcp/${id}`, "PUT", patch),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: mcpKeys.servers() });
    },
  });
}

/** Delete an MCP server. */
export function useDeleteMcpServer() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiSend<void>(`/api/mcp/${id}`, "DELETE"),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: mcpKeys.servers() });
    },
  });
}
