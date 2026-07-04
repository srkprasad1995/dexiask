"use client";

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type DragEvent,
  type FormEvent,
  type KeyboardEvent,
} from "react";
import { ArrowUp, Paperclip, Square, X } from "lucide-react";
import type { FileUIPart } from "ai";
import type { ChatStatus } from "ai";

import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";

/**
 * The chat composer: a raised input with file attach (drag/drop + paste), a
 * send button, and a stop button while a turn streams. Uploads go through the
 * BFF (/api/upload); the returned same-origin URL becomes a `file` part.
 */
export function Composer({
  status,
  conversationId,
  placeholder,
  onSend,
  onStop,
  onUploadBucketChange,
}: {
  status: ChatStatus;
  /** Current conversation ID — passed to the upload endpoint for non-first turns. */
  conversationId?: string | null;
  /** Optional override for the idle-state textarea placeholder. */
  placeholder?: string;
  onSend: (text: string, files: FileUIPart[]) => void;
  onStop: () => void;
  /** Called once on mount with the stable uploadBucket UUID. */
  onUploadBucketChange?: (bucket: string) => void;
}) {
  const [value, setValue] = useState("");
  const [files, setFiles] = useState<FileUIPart[]>([]);
  const [uploading, setUploading] = useState(false);
  const [dragOver, setDragOver] = useState(false);

  const fileInputRef = useRef<HTMLInputElement>(null);
  // Stable upload bucket for this composer instance — survives re-renders.
  const uploadBucketRef = useRef(
    typeof crypto !== "undefined" ? crypto.randomUUID() : "fallback-bucket",
  );

  const busy = status === "submitted" || status === "streaming";

  // Notify parent of the uploadBucket once on mount.
  useEffect(() => {
    onUploadBucketChange?.(uploadBucketRef.current);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Upload a list of File objects and append their refs to state.
  const uploadFiles = useCallback(
    async (fileList: File[]) => {
      if (fileList.length === 0) return;
      setUploading(true);
      try {
        const results = await Promise.all(
          fileList.map(async (file) => {
            const fd = new FormData();
            fd.append("file", file);
            if (conversationId) {
              fd.append("conversationId", conversationId);
            } else {
              fd.append("uploadBucket", uploadBucketRef.current);
            }
            const res = await fetch("/api/upload", { method: "POST", body: fd });
            if (!res.ok) throw new Error(`Upload failed for ${file.name}`);
            const data = (await res.json()) as {
              id: string;
              url: string;
              mediaType: string;
              filename: string;
            };
            const part: FileUIPart = {
              type: "file",
              url: data.url,
              mediaType: data.mediaType,
              filename: data.filename,
            };
            return part;
          }),
        );
        setFiles((prev) => [...prev, ...results]);
      } catch (err) {
        console.error("upload error", err);
      } finally {
        setUploading(false);
      }
    },
    [conversationId],
  );

  function handleFileInput(e: React.ChangeEvent<HTMLInputElement>) {
    const picked = Array.from(e.target.files ?? []);
    e.target.value = ""; // reset so the same file can be re-selected
    uploadFiles(picked);
  }

  function handleDragOver(e: DragEvent<HTMLFormElement>) {
    e.preventDefault();
    setDragOver(true);
  }

  function handleDragLeave() {
    setDragOver(false);
  }

  function handleDrop(e: DragEvent<HTMLFormElement>) {
    e.preventDefault();
    setDragOver(false);
    uploadFiles(Array.from(e.dataTransfer.files));
  }

  function handlePaste(e: React.ClipboardEvent<HTMLTextAreaElement>) {
    const pasted = Array.from(e.clipboardData.files);
    if (pasted.length > 0) {
      e.preventDefault();
      uploadFiles(pasted);
    }
  }

  function removeFile(url: string) {
    setFiles((prev) => prev.filter((f) => f.url !== url));
  }

  function submit() {
    const text = value.trim();
    if ((!text && files.length === 0) || uploading || busy) return;
    onSend(text, files);
    setValue("");
    setFiles([]);
  }

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    submit();
  }

  function handleKeyDown(e: KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      submit();
    }
  }

  const hasContent = value.trim() !== "" || files.length > 0;
  const canSubmit = hasContent && !uploading && !busy;

  return (
    <form
      onSubmit={handleSubmit}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      className={`relative rounded-2xl border bg-card shadow-dx-md transition-colors focus-within:border-primary/50 focus-within:ring-1 focus-within:ring-primary/20 ${dragOver ? "border-primary ring-1 ring-primary" : ""}`}
    >
      {/* Selected file previews */}
      {files.length > 0 && (
        <div className="flex flex-wrap gap-2 px-4 pt-3">
          {files.map((f) => (
            <div key={f.url} className="relative">
              {f.mediaType.startsWith("image/") ? (
                <div className="relative h-16 w-16 overflow-hidden rounded-lg border bg-muted">
                  {/* eslint-disable-next-line @next/next/no-img-element */}
                  <img
                    src={f.url}
                    alt={f.filename ?? "attachment"}
                    className="h-full w-full object-cover"
                  />
                </div>
              ) : (
                <div className="flex items-center gap-1.5 rounded-lg border bg-muted/50 px-2.5 py-1.5 text-xs">
                  <span className="max-w-[120px] truncate font-medium">
                    {f.filename ?? "file"}
                  </span>
                </div>
              )}
              <button
                type="button"
                onClick={() => removeFile(f.url)}
                className="absolute -top-1.5 -right-1.5 flex h-4 w-4 items-center justify-center rounded-full bg-foreground text-background shadow"
                title="Remove"
              >
                <X className="h-2.5 w-2.5" />
              </button>
            </div>
          ))}
          {uploading && (
            <div className="flex h-16 w-16 animate-pulse items-center justify-center rounded-lg border bg-muted text-xs text-muted-foreground">
              …
            </div>
          )}
        </div>
      )}

      {/* Textarea */}
      <Textarea
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={handleKeyDown}
        onPaste={handlePaste}
        placeholder={placeholder ?? "Ask Dexiask to explain, find, or explore…"}
        rows={1}
        className="max-h-48 min-h-[44px] resize-none rounded-2xl border-0 bg-transparent px-4 py-3 text-sm shadow-none focus-visible:ring-0"
      />

      {/* Control bar — a normal flow row below the textarea. */}
      <div className="flex items-center gap-1 px-2 pb-2">
        <div className="ml-auto flex items-center gap-1">
          {/* Hidden file input */}
          <input
            ref={fileInputRef}
            type="file"
            multiple
            className="hidden"
            onChange={handleFileInput}
            aria-label="Attach files"
          />
          <Button
            type="button"
            size="icon"
            variant="ghost"
            aria-label="Attach files"
            onClick={() => fileInputRef.current?.click()}
            className="h-9 w-9 rounded-full text-muted-foreground hover:text-foreground"
          >
            <Paperclip className="h-4 w-4" />
          </Button>

          {busy ? (
            <Button
              type="button"
              size="icon"
              variant="secondary"
              aria-label="Stop generating"
              onClick={onStop}
              className="h-9 w-9 rounded-full"
            >
              <Square className="h-4 w-4 fill-current" />
            </Button>
          ) : (
            <Button
              type="submit"
              size="icon"
              aria-label="Send message"
              disabled={!canSubmit}
              className="h-9 w-9 rounded-full"
            >
              <ArrowUp className="h-4 w-4" />
            </Button>
          )}
        </div>
      </div>
    </form>
  );
}
