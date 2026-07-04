"use client";

import { useId } from "react";

import { cn } from "@/lib/utils";

type DexiaskAvatarState = "default" | "thinking" | "success";

interface DexiaskAvatarProps {
  size?: number;
  state?: DexiaskAvatarState;
  className?: string;
  round?: boolean;
}

/**
 * The Dexiask mark — an "ask" speech bubble with an AI spark on a warm orange
 * gradient tile. Inline SVG so it inherits no external assets; `state` animates
 * the assistant avatar: an idle spark, typing dots while thinking, a check on
 * success.
 */
export function DexiaskAvatar({
  size = 28,
  state = "default",
  className,
  round = true,
}: DexiaskAvatarProps) {
  const gid = useId();
  const inset = round ? 0 : 4;
  const dim = round ? 80 : 72;
  const rx = round ? 40 : 18;
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 80 80"
      className={cn("shrink-0", className)}
      aria-label="Dexiask"
      role="img"
    >
      <defs>
        <linearGradient id={gid} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0" stopColor="#EF7A44" />
          <stop offset="1" stopColor="#C6471C" />
        </linearGradient>
      </defs>
      <rect x={inset} y={inset} width={dim} height={dim} rx={rx} fill={`url(#${gid})`} />

      {/* speech bubble (tail + body) */}
      <path d="M28 46 L25 59 L41 47 Z" fill="#fff" fillOpacity={0.96} />
      <rect x={16} y={19} width={48} height={30} rx={11} fill="#fff" fillOpacity={0.96} />

      {state === "default" && (
        <>
          <path
            d="M34.8 27.5 A6.2 6.2 0 1 1 40.5 36.2 V39"
            fill="none"
            stroke="#C6471C"
            strokeWidth={4}
            strokeLinecap="round"
          />
          <circle cx={40.5} cy={44} r={2.4} fill="#C6471C" />
        </>
      )}

      {state === "thinking" && (
        <g fill="#C6471C">
          <circle cx={31} cy={34} r={3.4} opacity={0.4} />
          <circle cx={40} cy={34} r={3.4} opacity={0.7} />
          <circle cx={49} cy={34} r={3.4} />
        </g>
      )}

      {state === "success" && (
        <path
          d="M31 34 L37.5 40.5 L50 27.5"
          fill="none"
          stroke="#C6471C"
          strokeWidth={5}
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      )}
    </svg>
  );
}
