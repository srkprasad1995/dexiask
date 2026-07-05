"use client";

import { useState } from "react";
import { Brain, FileText, Loader2, Sparkles } from "lucide-react";
import { toast } from "sonner";

import {
  useConsolidate,
  useMemoryEntries,
  useMemoryScopes,
  useWorkingMemory,
  type MemoryScope,
} from "@/lib/api/memory";
import { useIsAdmin } from "@/lib/auth/use-user";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";

function scopeKey(s: MemoryScope): string {
  return `${s.scope}:${s.id}`;
}

/** Consolidated entries + raw observations for the selected scope. */
function ScopeDetail({ scope }: { scope: MemoryScope }) {
  const entries = useMemoryEntries(scope.scope, scope.id);
  const working = useWorkingMemory(scope.scope, scope.id);
  const entryList = entries.data ?? [];
  const workingList = working.data ?? [];

  return (
    <div className="space-y-4">
      <section className="space-y-2">
        <h3 className="flex items-center gap-2 text-sm font-semibold">
          <Brain className="h-4 w-4 text-muted-foreground" />
          Consolidated memory
        </h3>
        {entries.isLoading && <Skeleton className="h-16 w-full" />}
        {!entries.isLoading && entryList.length === 0 && (
          <p className="text-sm text-muted-foreground">
            No consolidated entries yet — they appear after a dream run.
          </p>
        )}
        <div className="space-y-2">
          {entryList.map((e) => (
            <div
              key={`${e.topic}/${e.entry}`}
              className="rounded-xl border bg-card p-3 shadow-dx-sm"
            >
              <div className="flex items-center gap-2">
                <span className="font-plex-mono text-xs font-medium">
                  {e.topic}/{e.entry}
                </span>
              </div>
              <p className="mt-1 text-xs text-muted-foreground">{e.preview}</p>
            </div>
          ))}
        </div>
      </section>

      <section className="space-y-2">
        <h3 className="flex items-center gap-2 text-sm font-semibold">
          <FileText className="h-4 w-4 text-muted-foreground" />
          Working observations
        </h3>
        {!working.isLoading && workingList.length === 0 && (
          <p className="text-sm text-muted-foreground">No pending observations.</p>
        )}
        <div className="space-y-1">
          {workingList.map((f) => (
            <div
              key={f.filename}
              className="flex items-center gap-2 rounded-lg border bg-muted/30 px-3 py-2 text-xs"
            >
              <span className="font-plex-mono">{f.date}</span>
              <Badge variant="secondary">{f.entry_count}</Badge>
              <span className="min-w-0 flex-1 truncate text-muted-foreground">
                {f.preview}
              </span>
            </div>
          ))}
        </div>
      </section>
    </div>
  );
}

export function MemoryView() {
  const { data: scopes, isLoading, isError } = useMemoryScopes();
  const consolidate = useConsolidate();
  const isAdmin = useIsAdmin();
  const [selected, setSelected] = useState<string>("");

  const scopeList = scopes ?? [];
  const current =
    scopeList.find((s) => scopeKey(s) === selected) ?? scopeList[0];

  function runConsolidation() {
    consolidate.mutate(undefined, {
      onSuccess: () => toast.success("Consolidation started"),
      onError: (err) => toast.error(err.message || "Failed to start"),
    });
  }

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto w-full max-w-3xl space-y-6 px-4 py-8">
        <header className="flex items-start justify-between gap-4">
          <div className="space-y-1">
            <h1 className="text-lg font-semibold tracking-tight">Memory</h1>
            <p className="text-sm text-muted-foreground">
              What the assistant remembers about you and your repos. The dream
              judge consolidates observations into durable memory.
            </p>
          </div>
          {isAdmin && (
            <Button
              onClick={runConsolidation}
              disabled={consolidate.isPending}
              className="shrink-0 gap-1.5"
            >
              {consolidate.isPending ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Sparkles className="h-4 w-4" />
              )}
              Consolidate now
            </Button>
          )}
        </header>

        {isError && (
          <p className="text-sm text-muted-foreground">
            Memory service is not available.
          </p>
        )}
        {isLoading && <Skeleton className="h-8 w-full" />}
        {!isLoading && scopeList.length === 0 && !isError && (
          <p className="text-sm text-muted-foreground">
            No memory yet. Chat with the assistant and it will start recording
            observations.
          </p>
        )}

        {scopeList.length > 0 && (
          <div className="flex flex-wrap gap-1.5">
            {scopeList.map((s) => {
              const active = current && scopeKey(s) === scopeKey(current);
              return (
                <Button
                  key={scopeKey(s)}
                  size="sm"
                  variant={active ? "default" : "outline"}
                  onClick={() => setSelected(scopeKey(s))}
                >
                  {s.scope === "global" ? "global" : `${s.scope}: ${s.label}`}
                </Button>
              );
            })}
          </div>
        )}

        {current && <ScopeDetail scope={current} />}
      </div>
    </div>
  );
}
