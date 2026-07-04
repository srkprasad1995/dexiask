"use client";

import { useEffect, useRef, useState } from "react";
import { Check, Code2, Copy, Download, Eye, Maximize2, X } from "lucide-react";
import type { CustomRendererProps } from "streamdown";

import { buildSrcdoc, HTML_PREVIEW_HEIGHT_MSG } from "@/lib/html-preview";
import { cn } from "@/lib/utils";

const HEIGHT_MSG = HTML_PREVIEW_HEIGHT_MSG;

function IconButton({
  onClick,
  title,
  children,
}: {
  onClick: () => void;
  title: string;
  children: React.ReactNode;
}) {
  return (
    <button
      onClick={onClick}
      title={title}
      className="flex h-6 w-6 items-center justify-center rounded text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
    >
      {children}
    </button>
  );
}

/**
 * Renders a ```html fenced block as a live sandboxed preview.
 *
 * Features:
 *   • Preview / Code toggle
 *   • Copy source to clipboard
 *   • Download as .html file
 *   • Fullscreen modal (Escape to close)
 *   • Auto-height via ResizeObserver → postMessage (clamped 80–600 px)
 *
 * Security: sandbox="allow-scripts" — scripts run in a sandboxed unique
 * origin with no access to parent cookies, localStorage, or navigation.
 */
export function HtmlPreview({ code, isIncomplete }: CustomRendererProps) {
  const [view, setView] = useState<"preview" | "code">("preview");
  const [height, setHeight] = useState(200);
  const [fullscreen, setFullscreen] = useState(false);
  const [copied, setCopied] = useState(false);
  const iframeRef = useRef<HTMLIFrameElement>(null);
  const fsIframeRef = useRef<HTMLIFrameElement>(null);

  // Auto-height: listen for postMessage from the inline iframe.
  useEffect(() => {
    function onMessage(e: MessageEvent) {
      if (
        (e.source === iframeRef.current?.contentWindow ||
          e.source === fsIframeRef.current?.contentWindow) &&
        e.data?.type === HEIGHT_MSG &&
        typeof e.data.height === "number"
      ) {
        setHeight(Math.min(Math.max(Math.ceil(e.data.height) + 16, 80), 600));
      }
    }
    window.addEventListener("message", onMessage);
    return () => window.removeEventListener("message", onMessage);
  }, []);

  // Close fullscreen on Escape.
  useEffect(() => {
    if (!fullscreen) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setFullscreen(false);
    }
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [fullscreen]);

  function handleCopy() {
    navigator.clipboard.writeText(code).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }

  function handleDownload() {
    const blob = new Blob([buildSrcdoc(code)], { type: "text/html" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "preview.html";
    a.click();
    URL.revokeObjectURL(url);
  }

  const srcdoc = isIncomplete ? "" : buildSrcdoc(code);

  const toolbar = (
    <div className="flex items-center justify-between border-b bg-muted/40 px-3 py-1.5">
      <span className="text-xs font-medium text-muted-foreground">
        HTML Preview
      </span>
      <div className="flex items-center gap-1">
        {/* Preview / Code toggle */}
        <div className="flex items-center gap-0.5 rounded-md bg-muted p-0.5">
          <button
            onClick={() => setView("preview")}
            className={cn(
              "flex items-center gap-1 rounded px-2 py-0.5 text-xs transition-colors",
              view === "preview"
                ? "bg-background text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            <Eye className="h-3 w-3" />
            Preview
          </button>
          <button
            onClick={() => setView("code")}
            className={cn(
              "flex items-center gap-1 rounded px-2 py-0.5 text-xs transition-colors",
              view === "code"
                ? "bg-background text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            <Code2 className="h-3 w-3" />
            Code
          </button>
        </div>

        {/* Action buttons */}
        <div className="ml-0.5 flex items-center gap-0.5 border-l pl-1">
          <IconButton onClick={handleCopy} title="Copy HTML">
            {copied ? (
              <Check className="h-3.5 w-3.5 text-green-500" />
            ) : (
              <Copy className="h-3.5 w-3.5" />
            )}
          </IconButton>
          <IconButton onClick={handleDownload} title="Download as HTML file">
            <Download className="h-3.5 w-3.5" />
          </IconButton>
          {!fullscreen && (
            <IconButton
              onClick={() => setFullscreen(true)}
              title="Fullscreen (Esc to close)"
            >
              <Maximize2 className="h-3.5 w-3.5" />
            </IconButton>
          )}
        </div>
      </div>
    </div>
  );

  const body = (expand: boolean) =>
    view === "preview" ? (
      isIncomplete ? (
        <div className="flex h-16 items-center justify-center text-xs text-muted-foreground">
          <span className="animate-pulse">Rendering…</span>
        </div>
      ) : (
        <iframe
          ref={expand ? fsIframeRef : iframeRef}
          srcDoc={srcdoc}
          sandbox="allow-scripts"
          title="HTML Preview"
          className={cn(
            "w-full border-0",
            expand ? "flex-1" : "transition-[height] duration-150",
          )}
          style={expand ? undefined : { height }}
        />
      )
    ) : (
      <pre
        className={cn(
          "overflow-x-auto p-3 text-xs leading-relaxed text-foreground",
          expand && "flex-1 overflow-y-auto",
        )}
      >
        <code>{code}</code>
      </pre>
    );

  return (
    <>
      {/* Inline card */}
      <div className="my-3 overflow-hidden rounded-lg border bg-background">
        {toolbar}
        {body(false)}
      </div>

      {/* Fullscreen modal */}
      {fullscreen && (
        <div className="fixed inset-0 z-50 flex flex-col bg-background">
          <div className="flex items-center justify-between border-b bg-muted/40 px-3 py-1.5">
            <span className="text-xs font-medium text-muted-foreground">
              HTML Preview — Fullscreen
            </span>
            <div className="flex items-center gap-1">
              <div className="flex items-center gap-0.5 rounded-md bg-muted p-0.5">
                <button
                  onClick={() => setView("preview")}
                  className={cn(
                    "flex items-center gap-1 rounded px-2 py-0.5 text-xs transition-colors",
                    view === "preview"
                      ? "bg-background text-foreground shadow-sm"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  <Eye className="h-3 w-3" />
                  Preview
                </button>
                <button
                  onClick={() => setView("code")}
                  className={cn(
                    "flex items-center gap-1 rounded px-2 py-0.5 text-xs transition-colors",
                    view === "code"
                      ? "bg-background text-foreground shadow-sm"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  <Code2 className="h-3 w-3" />
                  Code
                </button>
              </div>
              <div className="ml-0.5 flex items-center gap-0.5 border-l pl-1">
                <IconButton onClick={handleCopy} title="Copy HTML">
                  {copied ? (
                    <Check className="h-3.5 w-3.5 text-green-500" />
                  ) : (
                    <Copy className="h-3.5 w-3.5" />
                  )}
                </IconButton>
                <IconButton
                  onClick={handleDownload}
                  title="Download as HTML file"
                >
                  <Download className="h-3.5 w-3.5" />
                </IconButton>
                <IconButton
                  onClick={() => setFullscreen(false)}
                  title="Exit fullscreen (Esc)"
                >
                  <X className="h-3.5 w-3.5" />
                </IconButton>
              </div>
            </div>
          </div>
          {body(true)}
        </div>
      )}
    </>
  );
}
