import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  reactStrictMode: true,
  agentRules: false,
  // Emits .next/standalone, which the runtime stage copies. Without it the image has no
  // server-only dependency tree to run from.
  output: "standalone",
};

export default nextConfig;
