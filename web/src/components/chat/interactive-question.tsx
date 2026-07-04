"use client";

import { useMemo, useState } from "react";
import { Check, HelpCircle } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";

/**
 * Renders an `AskChoice` tool call as clickable choices. The question
 * payload lives in the tool *input* (already streamed by the engine); the user's
 * selection is sent back as an ordinary next-turn user message via `onAnswer`.
 *
 * Supports single-select, multi-select, and yes/no confirm (a 2-option
 * single-select), each with an always-available "Other…" free-text escape.
 */

const OTHER = "__other__";

interface QuestionOption {
  label: string;
  description?: string;
}

interface Question {
  question: string;
  header: string;
  multiSelect: boolean;
  options: QuestionOption[];
}

/** Defensively parse the tool input into a list of questions. */
function parseQuestions(input: unknown): Question[] {
  const raw = (input as { questions?: unknown })?.questions;
  if (!Array.isArray(raw)) return [];
  return raw
    .map((q): Question | null => {
      if (!q || typeof q !== "object") return null;
      const obj = q as Record<string, unknown>;
      const options = Array.isArray(obj.options)
        ? obj.options
            .map((o): QuestionOption | null => {
              if (
                o &&
                typeof o === "object" &&
                typeof (o as { label?: unknown }).label === "string"
              ) {
                const oo = o as { label: string; description?: unknown };
                return {
                  label: oo.label,
                  description:
                    typeof oo.description === "string"
                      ? oo.description
                      : undefined,
                };
              }
              if (typeof o === "string") return { label: o };
              return null;
            })
            .filter((o): o is QuestionOption => o !== null)
        : [];
      if (typeof obj.question !== "string" || options.length === 0) return null;
      return {
        question: obj.question,
        header: typeof obj.header === "string" ? obj.header : "",
        multiSelect: obj.multiSelect === true,
        options,
      };
    })
    .filter((q): q is Question => q !== null);
}

/** Selected labels (+ optional Other text) per question index. */
type Selections = Record<number, { labels: Set<string>; other: string }>;

function formatAnswer(questions: Question[], sel: Selections): string {
  return questions
    .map((q, i) => {
      const s = sel[i];
      const picks: string[] = [];
      for (const label of q.options.map((o) => o.label)) {
        if (s?.labels.has(label)) picks.push(label);
      }
      if (s?.labels.has(OTHER) && s.other.trim()) picks.push(s.other.trim());
      const label = q.header || q.question;
      return `- ${label}: ${picks.join(", ")}`;
    })
    .join("\n");
}

export function InteractiveQuestion({
  inputs,
  interactive,
  onAnswer,
}: {
  /** Tool inputs of every AskChoice call in the message — flattened into one
   * card so all questions share a single "Submit all". */
  inputs: unknown[];
  interactive: boolean;
  onAnswer?: (text: string) => void;
}) {
  const questions = useMemo(
    () => inputs.flatMap((input) => parseQuestions(input)),
    [inputs],
  );
  const [sel, setSel] = useState<Selections>({});
  const [submitted, setSubmitted] = useState(false);

  if (questions.length === 0) return null;

  const locked = submitted || !interactive;

  function toggle(qi: number, q: Question, label: string) {
    if (locked) return;
    setSel((prev) => {
      const cur = prev[qi]?.labels ?? new Set<string>();
      const next = new Set(cur);
      if (q.multiSelect) {
        if (next.has(label)) next.delete(label);
        else next.add(label);
      } else {
        // Single-select: replace the choice.
        next.clear();
        next.add(label);
      }
      return { ...prev, [qi]: { labels: next, other: prev[qi]?.other ?? "" } };
    });
  }

  function setOther(qi: number, text: string) {
    setSel((prev) => ({
      ...prev,
      [qi]: { labels: prev[qi]?.labels ?? new Set<string>(), other: text },
    }));
  }

  // Every question must have at least one valid selection.
  const complete = questions.every((_, i) => {
    const s = sel[i];
    if (!s || s.labels.size === 0) return false;
    if (s.labels.has(OTHER) && !s.other.trim()) return false;
    return true;
  });

  function submit() {
    if (!complete || locked || !onAnswer) return;
    onAnswer(formatAnswer(questions, sel));
    setSubmitted(true);
  }

  return (
    <div className="my-2 space-y-4 rounded-lg border bg-card p-3 shadow-dx-sm">
      {questions.map((q, qi) => {
        const s = sel[qi];
        return (
          <div key={qi} className="space-y-2">
            <div className="flex items-center gap-2">
              <HelpCircle className="h-4 w-4 shrink-0 text-primary" />
              {q.header && (
                <span className="rounded bg-primary/10 px-1.5 py-0.5 text-xs font-medium text-primary">
                  {q.header}
                </span>
              )}
              <span className="text-sm font-medium">{q.question}</span>
            </div>
            <div className="flex flex-col gap-1.5">
              {q.options.map((opt, oi) => {
                const checked = !!s?.labels.has(opt.label);
                const letter = String.fromCharCode(65 + oi); // A, B, C…
                return (
                  <button
                    key={opt.label}
                    type="button"
                    disabled={locked}
                    onClick={() => toggle(qi, q, opt.label)}
                    className={cn(
                      "flex items-start gap-2 rounded-md border px-3 py-2 text-left text-sm transition-colors",
                      checked
                        ? "border-primary bg-primary/5"
                        : "bg-background hover:border-primary/40",
                      locked && "cursor-default opacity-80",
                    )}
                  >
                    <span
                      className={cn(
                        "mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center border",
                        q.multiSelect ? "rounded" : "rounded-full",
                        checked
                          ? "border-primary bg-primary text-primary-foreground"
                          : "border-input",
                      )}
                    >
                      {checked && <Check className="h-3 w-3" />}
                    </span>
                    <span className="min-w-0 flex-1">
                      <span className="font-medium">{opt.label}</span>
                      {opt.description && (
                        <span className="block text-xs text-muted-foreground">
                          {opt.description}
                        </span>
                      )}
                    </span>
                    {/* Mono shortcut letter (A/B/C…) — discovery hue when picked. */}
                    <span
                      className={cn(
                        "mt-0.5 shrink-0 rounded-full px-1.5 font-plex-mono text-[10px] leading-5 font-medium",
                        checked
                          ? "bg-dx-discovery-bg text-dx-discovery"
                          : "bg-muted text-muted-foreground",
                      )}
                    >
                      {letter}
                    </span>
                  </button>
                );
              })}
              {/* Always-available free-text escape. */}
              <button
                type="button"
                disabled={locked}
                onClick={() => toggle(qi, q, OTHER)}
                className={cn(
                  "flex items-center gap-2 rounded-md border px-3 py-2 text-left text-sm transition-colors",
                  s?.labels.has(OTHER)
                    ? "border-primary bg-primary/5"
                    : "bg-background hover:border-primary/40",
                  locked && "cursor-default opacity-80",
                )}
              >
                <span
                  className={cn(
                    "flex h-4 w-4 shrink-0 items-center justify-center border",
                    q.multiSelect ? "rounded" : "rounded-full",
                    s?.labels.has(OTHER)
                      ? "border-primary bg-primary text-primary-foreground"
                      : "border-input",
                  )}
                >
                  {s?.labels.has(OTHER) && <Check className="h-3 w-3" />}
                </span>
                <span className="font-medium">Other…</span>
              </button>
              {s?.labels.has(OTHER) && (
                <Textarea
                  autoFocus
                  disabled={locked}
                  value={s.other}
                  onChange={(e) => setOther(qi, e.target.value)}
                  placeholder="Type your answer…"
                  className="min-h-10 text-sm"
                />
              )}
            </div>
          </div>
        );
      })}

      {!locked && (
        <Button size="sm" disabled={!complete} onClick={submit}>
          {questions.length > 1 ? "Submit all" : "Submit"}
        </Button>
      )}
      {submitted && (
        <p className="text-xs text-muted-foreground">Answer sent.</p>
      )}
    </div>
  );
}
