import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";

import type { IndexerStatus } from "@/lib/api/indexer";

// Mock the indexer API module so the banner reads a controlled status without
// touching the network. Only `useIndexerStatus` is consumed by the banner.
const useIndexerStatus = vi.fn();
vi.mock("@/lib/api/indexer", () => ({
  useIndexerStatus: () => useIndexerStatus(),
}));

import { EmbeddingsBanner } from "@/components/indexer/indexer-view";

function withStatus(data: IndexerStatus | undefined) {
  useIndexerStatus.mockReturnValue({ data });
}

describe("EmbeddingsBanner", () => {
  beforeEach(() => {
    useIndexerStatus.mockReset();
  });

  it("warns and names every way to enable semantic search when it is off", () => {
    // No provider resolved: neither a hosted key nor the local sidecar is set.
    withStatus({
      status: "ready",
      embeddings: { provider: "", available: false },
    });
    render(<EmbeddingsBanner />);
    expect(screen.getByText(/using lexical search/i)).toBeTruthy();
    expect(screen.getByText("VOYAGE_API_KEY")).toBeTruthy();
    expect(screen.getByText("OPENAI_API_KEY")).toBeTruthy();
    expect(screen.getByText("COMPOSE_PROFILES=local")).toBeTruthy();
  });

  it("renders nothing when semantic search is available", () => {
    withStatus({
      status: "ready",
      embeddings: { provider: "voyage", available: true },
    });
    const { container } = render(<EmbeddingsBanner />);
    expect(container.firstChild).toBeNull();
  });

  it("renders nothing when the local sidecar supplies embeddings", () => {
    // Local mode: no API key at all, yet semantic search is on — the banner
    // must not nag about keys the user does not need.
    withStatus({
      status: "ready",
      embeddings: { provider: "ollama", available: true },
    });
    const { container } = render(<EmbeddingsBanner />);
    expect(container.firstChild).toBeNull();
  });

  it("renders nothing while status is still loading (no embeddings field)", () => {
    withStatus(undefined);
    const { container } = render(<EmbeddingsBanner />);
    expect(container.firstChild).toBeNull();
  });
});
