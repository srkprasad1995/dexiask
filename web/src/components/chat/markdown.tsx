"use client";

import { memo, useMemo } from "react";
import React from "react";
import {
  Streamdown,
  type MermaidErrorComponentProps,
  type CustomRenderer,
  type CustomRendererProps,
  type ControlsConfig,
  type ThemeInput,
} from "streamdown";
import { code } from "@streamdown/code";
import {
  createMermaidPlugin,
  type MermaidPluginOptions,
  type DiagramPlugin,
} from "@streamdown/mermaid";
import { math } from "@streamdown/math";
import { useTheme } from "next-themes";

import { cn } from "@/lib/utils";
import { HtmlPreview } from "@/components/chat/html-preview";

// Mermaid v11 beta diagram types whose parsers only accept ASCII — emojis and
// other non-ASCII characters in titles or labels cause a lexical parse error.
const ASCII_STRICT_DIAGRAMS = new Set([
  "xychart-beta",
  "sankey-beta",
  "kanban",
  "packet-beta",
]);

// Matches all Unicode emoji (presentation sequences + extended pictographics).
const EMOJI_RE = /\p{Emoji_Presentation}|\p{Extended_Pictographic}/gu;

function preprocessMermaid(source: string): string {
  const firstLine = source.trimStart().split("\n")[0].trim();
  if (!ASCII_STRICT_DIAGRAMS.has(firstLine)) return source;
  return source
    .split("\n")
    .map((line) => line.replace(EMOJI_RE, "").trimEnd())
    .join("\n");
}

// Wraps @streamdown/mermaid to apply preprocessing before every render call.
function createPreprocessedMermaid(opts?: MermaidPluginOptions): DiagramPlugin {
  const base = createMermaidPlugin(opts);
  return {
    ...base,
    getMermaid(config?) {
      const instance = base.getMermaid(config);
      return {
        ...instance,
        render(id: string, source: string) {
          return instance.render(id, preprocessMermaid(source));
        },
      };
    },
  };
}

const mermaidPlugin = createPreprocessedMermaid();

/**
 * Fallback shown when a mermaid block fails to render.
 * Shows the raw diagram source as a code block so the content is never lost.
 */
function MermaidError({ chart, retry }: MermaidErrorComponentProps) {
  return (
    <div className="my-3 overflow-hidden rounded-lg border border-destructive/20">
      <div className="flex items-center justify-between border-b border-destructive/20 bg-destructive/5 px-3 py-1.5">
        <span className="text-xs text-destructive/80">
          Diagram failed to render
        </span>
        <button
          onClick={retry}
          className="text-xs text-muted-foreground underline hover:text-foreground"
        >
          Retry
        </button>
      </div>
      <pre className="overflow-x-auto p-3 text-xs leading-relaxed text-foreground">
        <code>{chart}</code>
      </pre>
    </div>
  );
}

const htmlRenderer: CustomRenderer = {
  language: ["html", "htm"],
  component: HtmlPreview,
};

// Mermaid v11 diagram types that agents may emit as a direct fenced-block language
// (e.g. ```xychart-beta) rather than inside a ```mermaid block. This renderer
// wraps them as ```mermaid so the mermaid plugin handles rendering.
const MERMAID_DIALECTS = [
  "xychart-beta",
  "sankey-beta",
  "block-beta",
  "packet-beta",
  "architecture-beta",
  "kanban",
  "radar",
  "treemap",
  "venn",
  "ishikawa",
];

const MermaidDialect = memo(function MermaidDialect({
  code,
  language,
  isIncomplete,
}: {
  code: string;
  language: string;
  isIncomplete: boolean;
}) {
  if (isIncomplete) return null;
  // Re-inject the diagram type as the first line so mermaid's detector picks it up,
  // then render via the Markdown component (which routes ```mermaid to the plugin).
  return <Markdown>{`\`\`\`mermaid\n${language}\n${code}\n\`\`\``}</Markdown>;
});

const mermaidDialectRenderer: CustomRenderer = {
  language: MERMAID_DIALECTS,
  component: MermaidDialect as React.ComponentType<CustomRendererProps>,
};

// Stable identities for the Streamdown props that never change between renders.
// Passing fresh objects/arrays each render defeats Streamdown's internal block
// memoization, which makes already-rendered diagrams re-render (visible flicker).
const STREAMDOWN_PLUGINS = {
  code,
  mermaid: mermaidPlugin,
  math,
  renderers: [htmlRenderer, mermaidDialectRenderer],
};
const SHIKI_THEME: [ThemeInput, ThemeInput] = ["github-light", "github-dark"];

// Render diagrams INLINE (static), like the design — keep copy/download/
// fullscreen but disable the pan-zoom viewer. Its ResizeObserver auto-fit loop
// continuously re-fit the SVG, which made diagrams flicker and float detached
// from their cards. Stable identity so it never re-triggers a render.
const STREAMDOWN_CONTROLS: ControlsConfig = {
  mermaid: { copy: true, download: true, fullscreen: true, panZoom: false },
};

/**
 * Conservative HTML allow-list widening on top of Streamdown's defaults.
 *
 * Streamdown's default rehype pipeline is rehype-raw → rehype-sanitize →
 * rehype-harden, so raw HTML in markdown is already parsed and rendered, but
 * attributes are stripped to the GitHub-flavored sanitize schema. We re-admit
 * the safe, useful subset (class for structural styling, open for details) and
 * add a few semantic tags (figure, figcaption, caption) not in the default list.
 * Script, iframe, style, and event handlers remain blocked by rehype-harden.
 */
const allowedHtmlTags: Record<string, string[]> = {
  div: ["class"],
  span: ["class"],
  section: ["class"],
  figure: ["class"],
  figcaption: [],
  caption: [],
  details: ["open"],
  summary: [],
};

/**
 * Markdown renderer for AI responses.
 *
 * Streamdown handles streaming-aware parsing with block-level memoization so
 * only the last block re-renders per token. Plugins enable:
 *   • Mermaid diagrams (flowcharts, pie charts, sequence, graphs) via @streamdown/mermaid
 *   • KaTeX math ($...$ / $$...$$) via @streamdown/math
 *   • Shiki code highlighting via @streamdown/code (dual light/dark theme)
 *   • GFM tables, task lists, strikethrough — built in to Streamdown
 *   • Sanitized raw HTML — Streamdown's default rehype pipeline; widened via allowedTags
 *
 * Mermaid theme follows the resolved light/dark mode. The component IS memo'd so
 * unrelated parent re-renders (e.g. the chat list re-rendering when `useChat`
 * updates) don't re-render — and thus don't re-paint — already-rendered diagrams,
 * which was a visible flicker. Theme changes still propagate: `useTheme` is a
 * context hook, and memo never blocks context-driven re-renders. The plugin /
 * shikiTheme objects are hoisted to stable module constants so Streamdown's own
 * block memoization isn't defeated by fresh prop identities each render.
 */
export const Markdown = memo(function Markdown({
  children,
  className,
}: {
  children: string;
  className?: string;
}) {
  const { resolvedTheme } = useTheme();

  const mermaidOptions = useMemo(
    () => ({
      config: {
        theme: (resolvedTheme === "dark" ? "dark" : "default") as
          | "dark"
          | "default",
        // Pie slices on the built-in dark theme render near-black → invisible on
        // our dark canvas. Pin the pie palette to the lifecycle phase hues (and
        // legible label/stroke colors) so pies read clearly in both themes.
        // These vars are pie-specific; other diagram types are unaffected.
        themeVariables:
          resolvedTheme === "dark"
            ? {
                pie1: "#5bc8d6",
                pie2: "#7fb0ee",
                pie3: "#e6a93c",
                pie4: "#74c29a",
                pieStrokeColor: "#0a121b",
                pieOuterStrokeColor: "#26384a",
                pieTitleTextColor: "#dde6ee",
                pieSectionTextColor: "#0a121b",
                pieLegendTextColor: "#dde6ee",
              }
            : {
                pie1: "#0e7c8b",
                pie2: "#2c66bc",
                pie3: "#a9731a",
                pie4: "#2e7d52",
                pieStrokeColor: "#ffffff",
                pieOuterStrokeColor: "#cfd8e1",
                pieTitleTextColor: "#26333c",
                pieSectionTextColor: "#ffffff",
                pieLegendTextColor: "#26333c",
              },
      },
      errorComponent: MermaidError,
    }),
    [resolvedTheme],
  );

  return (
    <Streamdown
      // Re-mount when the resolved theme changes so Mermaid diagrams re-render
      // with the matching light/dark theme. Streamdown memoizes diagrams by
      // source, so without this a light-rendered diagram keeps its (dark) edge
      // strokes after switching to dark mode — invisible on the dark canvas.
      // The key only changes on theme toggle, never during scroll/stream.
      key={resolvedTheme ?? "light"}
      plugins={STREAMDOWN_PLUGINS}
      mermaid={mermaidOptions}
      controls={STREAMDOWN_CONTROLS}
      shikiTheme={SHIKI_THEME}
      allowedTags={allowedHtmlTags}
      normalizeHtmlIndentation
      className={cn(
        "max-w-none text-sm leading-relaxed [&_pre]:my-3 [&_pre]:rounded-lg [&_pre]:text-xs",
        // Top-align table cells so a single-line cell doesn't float to the middle
        // of a row whose neighbour wraps to multiple lines.
        "[&_td]:align-top [&_th]:align-top",
        className,
      )}
    >
      {children}
    </Streamdown>
  );
});
