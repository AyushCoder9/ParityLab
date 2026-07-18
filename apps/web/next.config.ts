import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  output: "standalone",
  reactStrictMode: true,
  transpilePackages: ["@paritylab/contracts", "@paritylab/ui"],
  turbopack: { root: new URL("../..", import.meta.url).pathname },
};

export default nextConfig;
