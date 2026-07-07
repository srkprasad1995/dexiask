import { ImageResponse } from "next/og";

export const alt = "Dexiask — chat with your codebase";
export const size = { width: 1200, height: 630 };
export const contentType = "image/png";

// Brand tokens (dark "drafting table" tier).
const SLATE = "#0a121b";
const INK = "#dde6ee";
const ORANGE = "#ef7a4f";
const MUTED = "#8aa0b4";

export default function Image() {
  return new ImageResponse(
    (
      <div
        style={{
          width: "100%",
          height: "100%",
          display: "flex",
          flexDirection: "column",
          justifyContent: "space-between",
          background: `radial-gradient(1200px 600px at 15% -10%, #12202e 0%, ${SLATE} 55%)`,
          padding: "72px 80px",
          fontFamily: "sans-serif",
        }}
      >
        <div style={{ display: "flex", alignItems: "center", gap: 20 }}>
          <div
            style={{
              width: 44,
              height: 44,
              borderRadius: 12,
              background: ORANGE,
            }}
          />
          <div style={{ color: INK, fontSize: 34, fontWeight: 700, letterSpacing: -0.5 }}>
            Dexiask
          </div>
        </div>

        <div style={{ display: "flex", flexDirection: "column", gap: 24 }}>
          <div
            style={{
              color: INK,
              fontSize: 68,
              fontWeight: 700,
              lineHeight: 1.05,
              letterSpacing: -1.5,
              maxWidth: 900,
            }}
          >
            Chat with your codebase
          </div>
          <div style={{ color: MUTED, fontSize: 32, lineHeight: 1.3, maxWidth: 820 }}>
            Open-source AI answers over your code, with semantic search.
          </div>
        </div>

        <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
          <div style={{ width: 56, height: 4, background: ORANGE, borderRadius: 2 }} />
          <div style={{ color: MUTED, fontSize: 26 }}>dexiask.com</div>
        </div>
      </div>
    ),
    { ...size },
  );
}
