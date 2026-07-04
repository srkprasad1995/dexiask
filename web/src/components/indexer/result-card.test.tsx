import { describe, it, expect } from "vitest";
import { render, screen } from "@testing-library/react";

import { ResultCard } from "@/components/indexer/indexer-view";

/**
 * ResultCard renders both code hits and generated domain-knowledge docs. Docs
 * (content_type "doc") show their title + a "doc" badge; code shows the path +
 * line and a "code" badge.
 */
describe("ResultCard", () => {
  it("renders a domain-doc result with its title and a doc badge", () => {
    render(
      <ResultCard
        result={{
          repo: "r",
          contentType: "doc",
          path: ".dexiask-docs/architecture",
          title: "System Architecture",
          score: 0.87,
          content: "how it fits together",
        }}
      />,
    );
    expect(screen.getByText("doc")).toBeTruthy();
    expect(screen.getByText(/System Architecture/)).toBeTruthy();
  });

  it("renders a code result with its path and a code badge", () => {
    render(
      <ResultCard
        result={{
          repo: "r",
          contentType: "code",
          path: "internal/server/server.go",
          startLine: 42,
          score: 0.91,
          content: "func NewServer() {}",
        }}
      />,
    );
    expect(screen.getByText("code")).toBeTruthy();
    expect(screen.getByText(/internal\/server\/server\.go/)).toBeTruthy();
  });
});
