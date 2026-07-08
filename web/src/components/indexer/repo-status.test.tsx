import { describe, it, expect } from "vitest";

import { repoStatusLabel, statusVariant } from "@/components/indexer/indexer-view";
import { isActivePhase, type Repo } from "@/lib/api/indexer";

/**
 * The per-repo badge shows a live phase (with an embedding percent) while a run
 * is active, and falls back to the plain indexed / not indexed state when idle.
 */
describe("repo status badge", () => {
  const base: Repo = { id: "r", indexed: false };

  it("shows the embedding phase with its percent", () => {
    expect(
      repoStatusLabel({ ...base, status: "embedding", percent: 78 }),
    ).toBe("embedding 78%");
  });

  it("shows an active phase without a percent when none is reported", () => {
    expect(repoStatusLabel({ ...base, status: "cloning" })).toBe("cloning");
  });

  it("falls back to indexed / not indexed when idle", () => {
    expect(repoStatusLabel({ ...base, indexed: true })).toBe("indexed");
    expect(repoStatusLabel({ ...base, indexed: false })).toBe("not indexed");
  });

  it("maps active phases to the amber build variant", () => {
    expect(statusVariant("cloning")).toBe("build");
    expect(statusVariant("docs")).toBe("build");
    expect(statusVariant("embedding")).toBe("build");
    expect(statusVariant("indexed")).toBe("done");
  });

  it("recognises the active phases", () => {
    expect(isActivePhase("embedding")).toBe(true);
    expect(isActivePhase("idle")).toBe(false);
    expect(isActivePhase(undefined)).toBe(false);
  });
});
