import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  reactStrictMode: true,
  transpilePackages: ["@paritylab/contracts", "@paritylab/ui"],
  turbopack: { root: new URL("../..", import.meta.url).pathname },
  // The e2e/CI harness and local dev both hit the dev server via 127.0.0.1.
  // Without this, Next's dev-origin check silently blocks the HMR websocket
  // upgrade (returns a plain HTTP response instead of 101), and the Turbopack
  // dev client never finishes hydrating client components as a result — pages
  // stay frozen on their SSR fallback with zero client-side JS ever running.
  allowedDevOrigins: ["127.0.0.1"],
};

export default nextConfig;
