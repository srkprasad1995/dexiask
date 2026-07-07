import type { Metadata } from "next";
import {
  Geist,
  Geist_Mono,
  IBM_Plex_Sans,
  IBM_Plex_Mono,
  IBM_Plex_Serif,
} from "next/font/google";
import "./globals.css";
import "streamdown/styles.css";
import "katex/dist/katex.min.css";

import { Providers } from "@/components/providers";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

// The "drafting table" type system: IBM Plex — ink / data / document.
const plexSans = IBM_Plex_Sans({
  variable: "--font-plex-sans",
  subsets: ["latin"],
  weight: ["400", "500", "600", "700"],
});

const plexMono = IBM_Plex_Mono({
  variable: "--font-plex-mono",
  subsets: ["latin"],
  weight: ["400", "500", "600"],
});

const plexSerif = IBM_Plex_Serif({
  variable: "--font-plex-serif",
  subsets: ["latin"],
  weight: ["400", "600"],
  style: ["normal", "italic"],
});

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL ?? "https://dexiask.com";

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: {
    default: "Dexiask — chat with your codebase",
    template: "%s · Dexiask",
  },
  description:
    "Dexiask is an open-source AI assistant that reads your codebase and answers questions with semantic code search. Sign in with GitHub, bring your own API keys, docker compose up, and go.",
  applicationName: "Dexiask",
  keywords: [
    "chat with your codebase",
    "AI code search",
    "semantic code search",
    "open source AI assistant",
    "codebase Q&A",
    "self-hosted AI",
    "Claude codebase agent",
    "Dexiask",
  ],
  authors: [{ name: "Dexiask" }],
  creator: "Dexiask",
  publisher: "Dexiask",
  alternates: {
    canonical: "/",
  },
  openGraph: {
    type: "website",
    siteName: "Dexiask",
    url: SITE_URL,
    title: "Dexiask — chat with your codebase",
    description:
      "An open-source AI assistant that reads your codebase and answers questions with semantic code search. Bring your own keys and self-host.",
    // OG image supplied by the generated app/opengraph-image route.
  },
  twitter: {
    card: "summary_large_image",
    title: "Dexiask — chat with your codebase",
    description:
      "Open-source AI assistant that reads your codebase and answers questions with semantic code search.",
    // Twitter image supplied by the generated app/twitter-image route.
  },
  robots: {
    index: true,
    follow: true,
    googleBot: {
      index: true,
      follow: true,
      "max-image-preview": "large",
      "max-snippet": -1,
    },
  },
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="en"
      suppressHydrationWarning
      className={`${geistSans.variable} ${geistMono.variable} ${plexSans.variable} ${plexMono.variable} ${plexSerif.variable} h-full antialiased`}
    >
      <body className="flex min-h-full flex-col">
        <Providers>{children}</Providers>
      </body>
    </html>
  );
}
