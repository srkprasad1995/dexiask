"use client";

import { useEffect, useRef, useState } from "react";
import {
  AlertCircle,
  CheckCircle2,
  ChevronRight,
  Loader2,
  Wrench,
} from "lucide-react";
import type { ToolUIPart, DynamicToolUIPart } from "ai";

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { cn } from "@/lib/utils";

type AnyToolPart = ToolUIPart | DynamicToolUIPart;

function rawToolName(part: AnyToolPart): string {
  if (part.type === "dynamic-tool") return part.toolName;
  // Static tool parts are typed `tool-<name>`.
  return part.type.replace(/^tool-/, "");
}

// Friendly labels for the engine's MCP tools (names arrive MCP-namespaced as
// `mcp__<server>__<tool>`). Keeps the chat readable instead of showing raw ids.
const FRIENDLY_TOOL_NAMES: Record<string, string> = {
  memory_write: "Memory",
  memory_view: "View memory",
  memory_search: "Search memory",
  Read: "Read file",
  Glob: "Find files",
  Grep: "Search code",
  WebSearch: "Web search",
  WebFetch: "Fetch page",
};

// Human labels for memory_write commands, shown as "Memory · <label>".
const MEMORY_COMMAND_LABELS: Record<string, string> = {
  observe: "record observation",
  create: "save entry",
  update: "update entry",
  delete: "archive entry",
  clear_working: "mark processed",
  list_scopes: "list scopes",
};

function toolName(part: AnyToolPart): string {
  let raw = rawToolName(part);
  // Strip the `mcp__<server>__` namespace prefix.
  if (raw.startsWith("mcp__")) {
    raw = raw.split("__").slice(2).join("__") || raw;
  }
  const label = FRIENDLY_TOOL_NAMES[raw] ?? raw.replace(/_/g, " ");

  // For memory writes, append the command so "Memory · record observation" reads
  // clearly instead of a bare "Memory".
  if (
    raw === "memory_write" &&
    "input" in part &&
    part.input &&
    typeof part.input === "object"
  ) {
    const cmd = (part.input as { command?: string }).command;
    if (cmd) return `${label} · ${MEMORY_COMMAND_LABELS[cmd] ?? cmd}`;
  }
  return label;
}

/** A concise representative argument for the tool-call header (mono), e.g. the
 * file path or search query — mirrors the design's `.toolcall .targ`. */
function toolArg(part: AnyToolPart): string {
  if (!("input" in part) || !part.input || typeof part.input !== "object") {
    return "";
  }
  const input = part.input as Record<string, unknown>;
  const PREFERRED = [
    "file_path",
    "path",
    "pattern",
    "query",
    "url",
    "command",
    "name",
  ];
  for (const k of PREFERRED) {
    if (typeof input[k] === "string" && input[k]) return input[k] as string;
  }
  const firstStr = Object.values(input).find(
    (v) => typeof v === "string" && v,
  );
  return typeof firstStr === "string" ? firstStr : "";
}

function prettyJson(value: unknown): string {
  if (value == null) return "";
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}

/**
 * Renders a single tool invocation as a collapsible card. The state machine
 * (input-streaming -> input-available -> output-available | output-error)
 * comes straight from the AI SDK part `state`.
 */
export function ToolCall({ part }: { part: AnyToolPart }) {
  const state = part.state;
  const running = state === "input-streaming" || state === "input-available";
  const errored = state === "output-error";

  // Expanded while the tool is running (live), auto-collapses on completion so
  // a finished turn isn't a wall of open tool cards. Manual toggle wins until
  // the next running→done transition.
  const [open, setOpen] = useState(running);
  const prevRunning = useRef(running);
  useEffect(() => {
    if (prevRunning.current !== running) {
      setOpen(running);
      prevRunning.current = running;
    }
  }, [running]);

  const name = toolName(part);
  const arg = toolArg(part);

  const StatusIcon = errored ? AlertCircle : running ? Loader2 : CheckCircle2;
  // Status reads in mono, colored by lifecycle hue: running=cyan, done=green,
  // error=red — matching the design's `.toolcall .tstatus`.
  const statusColor = errored
    ? "text-destructive"
    : running
      ? "text-dx-discovery"
      : "text-dx-done";

  return (
    <Collapsible
      open={open}
      onOpenChange={setOpen}
      className="my-2 rounded-lg border bg-card shadow-dx-sm"
    >
      <CollapsibleTrigger className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm">
        <Wrench className="h-4 w-4 shrink-0 text-muted-foreground" />
        <span className="shrink-0 font-medium">{name}</span>
        {arg && (
          <span className="min-w-0 truncate font-plex-mono text-xs text-muted-foreground">
            {arg}
          </span>
        )}
        <span
          className={cn(
            "ml-auto inline-flex shrink-0 items-center gap-1.5 font-plex-mono text-[10px] tracking-wide",
            statusColor,
          )}
        >
          <StatusIcon className={cn("h-3.5 w-3.5", running && "animate-spin")} />
          {errored ? "error" : running ? "running" : "done"}
        </span>
        <ChevronRight
          className={cn(
            "h-4 w-4 shrink-0 text-muted-foreground transition-transform",
            open && "rotate-90",
          )}
        />
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-2 px-3 pb-3">
        {"input" in part && part.input != null && (
          <div>
            <div className="mb-1 text-xs font-medium text-muted-foreground">
              Input
            </div>
            <pre className="overflow-x-auto rounded-md border bg-background p-2 text-xs">
              {prettyJson(part.input)}
            </pre>
          </div>
        )}
        {state === "output-available" && (
          <div>
            <div className="mb-1 text-xs font-medium text-muted-foreground">
              Output
            </div>
            <pre className="overflow-x-auto rounded-md border bg-background p-2 text-xs">
              {prettyJson(part.output)}
            </pre>
          </div>
        )}
        {state === "output-error" && (
          <div className="text-xs text-destructive">{part.errorText}</div>
        )}
      </CollapsibleContent>
    </Collapsible>
  );
}
