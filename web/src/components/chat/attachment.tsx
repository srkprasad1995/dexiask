"use client";

import { useEffect, useState } from "react";
import { Download, File, X } from "lucide-react";
import type { FileUIPart } from "ai";

import { cn } from "@/lib/utils";

/**
 * Renders a single file attachment.
 *
 * Images: shows a thumbnail; clicking opens a fullscreen lightbox (Esc to close).
 * Other files: shows a file-card with filename, size info, and a download link.
 */
export function Attachment({ part }: { part: FileUIPart }) {
  const isImage = part.mediaType.startsWith("image/");

  if (isImage) {
    return <ImageAttachment part={part} />;
  }
  return <FileCard part={part} />;
}

function ImageAttachment({ part }: { part: FileUIPart }) {
  const [lightbox, setLightbox] = useState(false);

  // Close lightbox on Escape.
  useEffect(() => {
    if (!lightbox) return;
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setLightbox(false);
    }
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [lightbox]);

  return (
    <>
      <button
        type="button"
        onClick={() => setLightbox(true)}
        className="relative overflow-hidden rounded-lg border bg-muted transition-opacity hover:opacity-90 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
        title={part.filename ?? "Attached image"}
      >
        {/* eslint-disable-next-line @next/next/no-img-element */}
        <img
          src={part.url}
          alt={part.filename ?? "Attached image"}
          className="max-h-48 max-w-48 object-cover"
          loading="lazy"
        />
      </button>

      {lightbox && (
        <div
          className="fixed inset-0 z-50 flex flex-col items-center justify-center bg-black/80 p-4"
          onClick={() => setLightbox(false)}
        >
          <button
            type="button"
            className="absolute top-4 right-4 flex h-8 w-8 items-center justify-center rounded-full bg-white/10 text-white transition-colors hover:bg-white/20"
            onClick={() => setLightbox(false)}
            title="Close (Esc)"
          >
            <X className="h-4 w-4" />
          </button>
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={part.url}
            alt={part.filename ?? "Attached image"}
            className="max-h-full max-w-full rounded-lg object-contain"
            onClick={(e) => e.stopPropagation()}
          />
          {part.filename && (
            <p className="mt-2 text-xs text-white/70">{part.filename}</p>
          )}
        </div>
      )}
    </>
  );
}

function FileCard({ part }: { part: FileUIPart }) {
  return (
    <a
      href={part.url}
      download={part.filename}
      className={cn(
        "inline-flex items-center gap-2 rounded-lg border bg-muted/50 px-3 py-2",
        "text-sm transition-colors hover:bg-muted",
      )}
      title={`Download ${part.filename ?? "file"}`}
    >
      <File className="h-4 w-4 shrink-0 text-muted-foreground" />
      <span className="min-w-0 truncate font-medium">
        {part.filename ?? "Attached file"}
      </span>
      <Download className="ml-1 h-3.5 w-3.5 shrink-0 text-muted-foreground" />
    </a>
  );
}
