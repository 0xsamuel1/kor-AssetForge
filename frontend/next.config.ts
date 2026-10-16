import type { NextConfig } from "next";
import path from "node:path";

const nextConfig: NextConfig = {
  // This is the alias installed by next-intl/plugin. Keeping it explicit
  // avoids loading the plugin's optional native message-extraction toolchain;
  // this application uses runtime JSON catalogs and does not use extraction.
  turbopack: {
    resolveAlias: {
      "next-intl/config": "./src/i18n/i18n.ts",
    },
  },
  // The service worker must never be cached by the browser or a CDN, or users
  // would keep running an old worker after a deploy.
  async headers() {
    return [
      {
        source: "/sw.js",
        headers: [
          { key: "Content-Type", value: "application/javascript; charset=utf-8" },
          { key: "Cache-Control", value: "no-cache, no-store, must-revalidate" },
          { key: "Service-Worker-Allowed", value: "/" },
        ],
      },
      {
        source: "/offline.html",
        headers: [{ key: "Cache-Control", value: "no-cache" }],
      },
    ];
  },
  webpack(config) {
    config.resolve ??= {};
    config.resolve.alias ??= {};
    config.resolve.alias["next-intl/config"] = path.resolve(process.cwd(), "src/i18n/i18n.ts");
    return config;
  },
};

export default nextConfig;
