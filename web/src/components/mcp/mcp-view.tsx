"use client";

import { useState, type FormEvent } from "react";
import { Loader2, Plug, Plus, Trash2, X } from "lucide-react";
import { toast } from "sonner";

import {
  useAddMcpServer,
  useDeleteMcpServer,
  useMcpServers,
  useUpdateMcpServer,
  type McpServer,
  type McpServerType,
} from "@/lib/api/mcp";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";

const MCP_TYPES: McpServerType[] = ["http", "sse"];

interface HeaderRow {
  key: string;
  value: string;
}

/** Collapse header rows into a map, dropping blank keys. */
function rowsToHeaders(rows: HeaderRow[]): Record<string, string> | undefined {
  const out: Record<string, string> = {};
  for (const { key, value } of rows) {
    const k = key.trim();
    if (k) out[k] = value;
  }
  return Object.keys(out).length ? out : undefined;
}

/** The "add an MCP server" form: name, type, url, and optional headers. */
function AddServerForm() {
  const [name, setName] = useState("");
  const [type, setType] = useState<McpServerType>("http");
  const [url, setUrl] = useState("");
  const [headers, setHeaders] = useState<HeaderRow[]>([]);
  const add = useAddMcpServer();

  function reset() {
    setName("");
    setType("http");
    setUrl("");
    setHeaders([]);
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    const trimmedName = name.trim();
    const trimmedUrl = url.trim();
    if (!trimmedName || !trimmedUrl) return;
    add.mutate(
      {
        name: trimmedName,
        type,
        url: trimmedUrl,
        headers: rowsToHeaders(headers),
      },
      {
        onSuccess: () => {
          toast.success(`Added ${trimmedName}`);
          reset();
        },
        onError: (err) => toast.error(err.message || "Failed to add server"),
      },
    );
  }

  return (
    <form
      onSubmit={onSubmit}
      className="space-y-3 rounded-xl border bg-card p-4 shadow-dx-sm"
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
        <div className="flex-1 space-y-1">
          <label className="font-plex-mono text-[10px] tracking-[0.15em] text-muted-foreground uppercase">
            Name
          </label>
          <Input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="github"
          />
        </div>
        <div className="space-y-1">
          <label className="font-plex-mono text-[10px] tracking-[0.15em] text-muted-foreground uppercase">
            Type
          </label>
          <select
            aria-label="Server type"
            value={type}
            onChange={(e) => setType(e.target.value as McpServerType)}
            className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm focus-visible:ring-1 focus-visible:ring-ring focus-visible:outline-none sm:w-24"
          >
            {MCP_TYPES.map((t) => (
              <option key={t} value={t}>
                {t}
              </option>
            ))}
          </select>
        </div>
        <div className="flex-[2] space-y-1">
          <label className="font-plex-mono text-[10px] tracking-[0.15em] text-muted-foreground uppercase">
            URL
          </label>
          <Input
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="https://mcp.example.com/mcp"
          />
        </div>
      </div>

      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <span className="font-plex-mono text-[10px] tracking-[0.15em] text-muted-foreground uppercase">
            Headers (optional)
          </span>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-7 gap-1 text-muted-foreground"
            onClick={() => setHeaders((h) => [...h, { key: "", value: "" }])}
          >
            <Plus className="h-3.5 w-3.5" />
            Add header
          </Button>
        </div>
        {headers.map((row, i) => (
          <div key={i} className="flex gap-2">
            <Input
              value={row.key}
              onChange={(e) =>
                setHeaders((h) =>
                  h.map((r, j) => (j === i ? { ...r, key: e.target.value } : r)),
                )
              }
              placeholder="Authorization"
              className="flex-1"
            />
            <Input
              value={row.value}
              onChange={(e) =>
                setHeaders((h) =>
                  h.map((r, j) =>
                    j === i ? { ...r, value: e.target.value } : r,
                  ),
                )
              }
              placeholder="Bearer …"
              className="flex-1"
            />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label="Remove header"
              onClick={() => setHeaders((h) => h.filter((_, j) => j !== i))}
            >
              <X className="h-4 w-4" />
            </Button>
          </div>
        ))}
      </div>

      <div className="flex justify-end">
        <Button type="submit" disabled={add.isPending} className="gap-1.5">
          {add.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Plus className="h-4 w-4" />
          )}
          Add MCP server
        </Button>
      </div>
    </form>
  );
}

/** One MCP server row: name, type badge, url, enabled toggle, delete. */
function ServerRow({ server }: { server: McpServer }) {
  const update = useUpdateMcpServer();
  const del = useDeleteMcpServer();

  return (
    <div className="flex items-center gap-3 rounded-xl border bg-card px-4 py-3 shadow-dx-sm">
      <Plug className="h-4 w-4 shrink-0 text-muted-foreground" />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium">{server.name}</div>
        <div className="truncate font-plex-mono text-xs text-muted-foreground">
          {server.url}
        </div>
      </div>
      <Badge variant="secondary" className="uppercase">
        {server.type}
      </Badge>
      <Button
        variant={server.enabled ? "default" : "outline"}
        size="sm"
        disabled={update.isPending}
        aria-pressed={server.enabled}
        onClick={() =>
          update.mutate(
            { id: server.id, enabled: !server.enabled },
            {
              onError: (err) =>
                toast.error(err.message || "Failed to update server"),
            },
          )
        }
      >
        {server.enabled ? "Enabled" : "Disabled"}
      </Button>
      <Button
        variant="ghost"
        size="icon"
        aria-label={`Delete ${server.name}`}
        disabled={del.isPending}
        onClick={() =>
          del.mutate(server.id, {
            onSuccess: () => toast.success(`Removed ${server.name}`),
            onError: (err) => toast.error(err.message || "Failed to delete"),
          })
        }
      >
        <Trash2 className="h-4 w-4" />
      </Button>
    </div>
  );
}

/** The list of registered MCP servers (with loading + empty states). */
function ServerList() {
  const { data, isLoading, isError, error } = useMcpServers();
  const servers = data?.mcpServers ?? [];

  return (
    <section className="space-y-3">
      <h2 className="flex items-center gap-2 text-sm font-semibold">
        <Plug className="h-4 w-4 text-muted-foreground" />
        Configured servers
      </h2>

      {isLoading ? (
        <div className="space-y-2">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </div>
      ) : isError ? (
        <p className="rounded-xl border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">
          Could not load MCP servers: {(error as Error)?.message}
        </p>
      ) : servers.length === 0 ? (
        <p className="rounded-xl border border-dashed bg-card/50 px-4 py-8 text-center text-sm text-muted-foreground">
          No MCP servers configured yet. Add one above to expose its tools to the
          agent.
        </p>
      ) : (
        <div className="space-y-2">
          {servers.map((server) => (
            <ServerRow key={server.id} server={server} />
          ))}
        </div>
      )}
    </section>
  );
}

export function McpView() {
  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto w-full max-w-3xl space-y-8 px-4 py-8">
        <header className="space-y-1">
          <h1 className="text-lg font-semibold tracking-tight">MCP servers</h1>
          <p className="text-sm text-muted-foreground">
            Connect custom MCP servers. Every enabled server is offered to the
            agent on each chat turn, alongside the built-in code indexer.
          </p>
        </header>

        <AddServerForm />
        <ServerList />
      </div>
    </div>
  );
}
