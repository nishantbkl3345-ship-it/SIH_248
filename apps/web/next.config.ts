import type { NextConfig } from "next";
import { env } from "./lib/env";

const nextConfig: NextConfig = {
  // The browser only ever talks to this origin; API calls are proxied to the
  // Go service so the session cookie stays first-party.
  async rewrites() {
    return [
      {
        source: "/api/v1/:path*",
        destination: `${env.API_URL}/api/v1/:path*`,
      },
    ];
  },
};

export default nextConfig;
