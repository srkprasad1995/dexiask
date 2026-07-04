import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  // Standalone output so the Docker image ships a minimal server bundle.
  output: "standalone",
  devIndicators: {
    position: "bottom-right",
  },
};

export default nextConfig;
