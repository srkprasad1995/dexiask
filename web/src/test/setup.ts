// Global test setup.
import { vi } from "vitest";

// happy-dom lacks a few browser globals that Streamdown / UI components touch.
// Provide minimal no-op polyfills so component render tests don't throw.
if (!("ResizeObserver" in globalThis)) {
  class ResizeObserver {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  (globalThis as Record<string, unknown>).ResizeObserver = ResizeObserver;
}

if (!("matchMedia" in globalThis)) {
  (globalThis as Record<string, unknown>).matchMedia = (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  });
}
