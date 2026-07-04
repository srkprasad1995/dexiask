"use client";

import { useState, type FormEvent } from "react";
import {
  Database,
  FileCode,
  Folder,
  GitBranch,
  KeyRound,
  Loader2,
  Plus,
  RefreshCw,
  Search,
} from "lucide-react";
import { toast } from "sonner";

import {
  useAddRepo,
  useGitTokenStatus,
  useIndexerStatus,
  useReindex,
  useRepos,
  useSearch,
  useSetGitToken,
  type Repo,
  type SearchResult,
} from "@/lib/api/indexer";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

function repoLabel(repo: Repo): string {
  return repo.url || repo.path || repo.id;
}

function statusVariant(status?: string): "done" | "build" | "destructive" | "secondary" {
  switch (status) {
    case "ready":
    case "indexed":
    case "idle":
      return "done";
    case "indexing":
    case "pending":
      return "build";
    case "error":
      return "destructive";
    default:
      return "secondary";
  }
}

/** The "add a repo" form: a git URL or a path under /workspace. */
function AddRepoForm() {
  const [id, setId] = useState("");
  const [source, setSource] = useState("");
  const addRepo = useAddRepo();

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    const trimmedId = id.trim();
    const trimmedSource = source.trim();
    if (!trimmedId || !trimmedSource) return;
    // A URL-ish source is treated as a git remote; anything else as a path.
    const isUrl = /^(https?:|git@|ssh:|git:)/.test(trimmedSource);
    addRepo.mutate(
      {
        id: trimmedId,
        ...(isUrl ? { url: trimmedSource } : { path: trimmedSource }),
      },
      {
        onSuccess: () => {
          toast.success(`Added ${trimmedId}`);
          setId("");
          setSource("");
        },
        onError: (err) => toast.error(err.message || "Failed to add repo"),
      },
    );
  }

  return (
    <form
      onSubmit={onSubmit}
      className="flex flex-col gap-2 rounded-xl border bg-card p-4 shadow-dx-sm sm:flex-row sm:items-end"
    >
      <div className="flex-1 space-y-1">
        <label className="font-plex-mono text-[10px] tracking-[0.15em] text-muted-foreground uppercase">
          Repo id
        </label>
        <Input
          value={id}
          onChange={(e) => setId(e.target.value)}
          placeholder="my-service"
        />
      </div>
      <div className="flex-[2] space-y-1">
        <label className="font-plex-mono text-[10px] tracking-[0.15em] text-muted-foreground uppercase">
          Git URL or /workspace path
        </label>
        <Input
          value={source}
          onChange={(e) => setSource(e.target.value)}
          placeholder="https://github.com/org/repo.git  ·  /workspace/repo"
        />
      </div>
      <Button type="submit" disabled={addRepo.isPending} className="gap-1.5">
        {addRepo.isPending ? (
          <Loader2 className="h-4 w-4 animate-spin" />
        ) : (
          <Plus className="h-4 w-4" />
        )}
        Add repo
      </Button>
    </form>
  );
}

/** One repo row: label, index status, and a per-repo reindex button. */
function RepoRow({ repo }: { repo: Repo }) {
  const reindex = useReindex();
  const isPath = !repo.url;
  const Icon = isPath ? Folder : GitBranch;

  return (
    <div className="flex items-center gap-3 rounded-xl border bg-card px-4 py-3 shadow-dx-sm">
      <Icon className="h-4 w-4 shrink-0 text-muted-foreground" />
      <div className="min-w-0 flex-1">
        <div className="truncate font-plex-mono text-sm">{repoLabel(repo)}</div>
        <div className="truncate text-xs text-muted-foreground">{repo.id}</div>
      </div>
      {typeof repo.chunks === "number" && (
        <span className="hidden font-plex-mono text-[11px] text-muted-foreground sm:inline">
          {repo.chunks.toLocaleString()} chunks
        </span>
      )}
      <Badge variant={statusVariant(repo.status)}>
        {repo.status ?? (repo.indexed ? "indexed" : "not indexed")}
      </Badge>
      <Button
        variant="outline"
        size="sm"
        className="gap-1.5"
        disabled={reindex.isPending}
        onClick={() =>
          reindex.mutate(
            { repo: repo.id },
            {
              onSuccess: () => toast.success(`Reindexing ${repo.id}`),
              onError: (err) => toast.error(err.message || "Reindex failed"),
            },
          )
        }
      >
        <RefreshCw
          className={cn("h-3.5 w-3.5", reindex.isPending && "animate-spin")}
        />
        Reindex
      </Button>
    </div>
  );
}

/** The list of registered repos (with loading + empty states). */
function RepoList() {
  const { data, isLoading, isError, error } = useRepos();
  const reindexAll = useReindex();
  const repos = data?.repos ?? [];

  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <Database className="h-4 w-4 text-muted-foreground" />
          Indexed repositories
        </h2>
        {repos.length > 0 && (
          <Button
            variant="ghost"
            size="sm"
            className="gap-1.5 text-muted-foreground"
            disabled={reindexAll.isPending}
            onClick={() =>
              reindexAll.mutate(
                {},
                {
                  onSuccess: () => toast.success("Reindexing all repos"),
                  onError: (err) =>
                    toast.error(err.message || "Reindex failed"),
                },
              )
            }
          >
            <RefreshCw
              className={cn(
                "h-3.5 w-3.5",
                reindexAll.isPending && "animate-spin",
              )}
            />
            Reindex all
          </Button>
        )}
      </div>

      {isLoading ? (
        <div className="space-y-2">
          <Skeleton className="h-16 w-full" />
          <Skeleton className="h-16 w-full" />
        </div>
      ) : isError ? (
        <p className="rounded-xl border border-destructive/30 bg-destructive/5 px-4 py-3 text-sm text-destructive">
          Could not load repositories: {(error as Error)?.message}
        </p>
      ) : repos.length === 0 ? (
        <p className="rounded-xl border border-dashed bg-card/50 px-4 py-8 text-center text-sm text-muted-foreground">
          No repositories indexed yet. Add one above to get started.
        </p>
      ) : (
        <div className="space-y-2">
          {repos.map((repo) => (
            <RepoRow key={repo.id} repo={repo} />
          ))}
        </div>
      )}
    </section>
  );
}

/** One semantic-search result card. */
function ResultCard({ result }: { result: SearchResult }) {
  return (
    <div className="overflow-hidden rounded-xl border bg-card shadow-dx-sm">
      <div className="flex items-center gap-2 border-b bg-muted/40 px-3 py-2">
        <FileCode className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1 truncate font-plex-mono text-xs">
          {result.repo ? `${result.repo}: ` : ""}
          {result.path}
          {result.startLine != null && `:${result.startLine}`}
        </span>
        {typeof result.score === "number" && (
          <span className="shrink-0 font-plex-mono text-[10px] tracking-wide text-muted-foreground">
            {result.score.toFixed(3)}
          </span>
        )}
      </div>
      {result.content && (
        <pre className="overflow-x-auto p-3 text-xs leading-relaxed">
          <code>{result.content}</code>
        </pre>
      )}
    </div>
  );
}

/** The semantic-search box + results. */
function SearchPanel() {
  const [query, setQuery] = useState("");
  const search = useSearch();
  const results = search.data?.results ?? [];

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    const q = query.trim();
    if (!q) return;
    search.mutate(
      { query: q, limit: 20 },
      { onError: (err) => toast.error(err.message || "Search failed") },
    );
  }

  return (
    <section className="space-y-3">
      <h2 className="flex items-center gap-2 text-sm font-semibold">
        <Search className="h-4 w-4 text-muted-foreground" />
        Semantic search
      </h2>
      <form onSubmit={onSubmit} className="flex gap-2">
        <Input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Describe what you're looking for…"
        />
        <Button type="submit" disabled={search.isPending} className="gap-1.5">
          {search.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Search className="h-4 w-4" />
          )}
          Search
        </Button>
      </form>

      {search.isSuccess && results.length === 0 && (
        <p className="text-sm text-muted-foreground">No matches found.</p>
      )}
      {results.length > 0 && (
        <div className="space-y-2">
          {results.map((r, i) => (
            <ResultCard key={i} result={r} />
          ))}
        </div>
      )}
    </section>
  );
}

/** A compact indexer health line. */
function StatusLine() {
  const { data } = useIndexerStatus();
  if (!data?.status) return null;
  return (
    <div className="flex items-center gap-2 text-xs text-muted-foreground">
      <span
        className={cn(
          "size-2 rounded-full",
          statusVariant(data.status) === "done"
            ? "bg-dx-done"
            : statusVariant(data.status) === "build"
              ? "bg-dx-build"
              : statusVariant(data.status) === "destructive"
                ? "bg-destructive"
                : "bg-muted-foreground",
        )}
      />
      <span className="font-plex-mono tracking-wide uppercase">
        {data.status}
      </span>
    </div>
  );
}

/**
 * The git access token panel. Lets the user provision a token so the indexer
 * can clone private repositories. The token is held server-side by the indexer
 * and never displayed — the UI only reflects whether one is configured.
 */
function GitTokenPanel() {
  const { data } = useGitTokenStatus();
  const setToken = useSetGitToken();
  const [token, setToken_] = useState("");
  const configured = data?.configured ?? false;

  function onSave(e: FormEvent) {
    e.preventDefault();
    const t = token.trim();
    if (!t) return;
    setToken.mutate(t, {
      onSuccess: () => {
        toast.success("Git token saved");
        setToken_("");
      },
      onError: (err) => toast.error(err.message || "Failed to save token"),
    });
  }

  function onClear() {
    setToken.mutate("", {
      onSuccess: () => {
        toast.success("Git token cleared");
        setToken_("");
      },
      onError: (err) => toast.error(err.message || "Failed to clear token"),
    });
  }

  return (
    <section className="space-y-3">
      <div className="flex items-center justify-between">
        <h2 className="flex items-center gap-2 text-sm font-semibold">
          <KeyRound className="h-4 w-4 text-muted-foreground" />
          Git access token
        </h2>
        <Badge variant={configured ? "done" : "secondary"}>
          {configured ? "configured" : "not configured"}
        </Badge>
      </div>
      <p className="text-xs text-muted-foreground">
        Used to clone private repositories. The token is stored server-side by
        the indexer and is never displayed.
      </p>
      <form onSubmit={onSave} className="flex gap-2">
        <Input
          type="password"
          value={token}
          onChange={(e) => setToken_(e.target.value)}
          placeholder={configured ? "Enter a new token to replace" : "ghp_…"}
          autoComplete="off"
        />
        <Button
          type="submit"
          disabled={setToken.isPending || !token.trim()}
          className="gap-1.5"
        >
          {setToken.isPending ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <KeyRound className="h-4 w-4" />
          )}
          Save
        </Button>
        {configured && (
          <Button
            type="button"
            variant="outline"
            disabled={setToken.isPending}
            onClick={onClear}
          >
            Clear
          </Button>
        )}
      </form>
    </section>
  );
}

export function IndexerView() {
  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto w-full max-w-3xl space-y-8 px-4 py-8">
        <header className="space-y-1">
          <div className="flex items-center justify-between">
            <h1 className="text-lg font-semibold tracking-tight">
              Code indexer
            </h1>
            <StatusLine />
          </div>
          <p className="text-sm text-muted-foreground">
            Register repositories, keep the semantic index fresh, and search your
            codebase by meaning.
          </p>
        </header>

        <AddRepoForm />
        <RepoList />
        <GitTokenPanel />
        <SearchPanel />
      </div>
    </div>
  );
}
