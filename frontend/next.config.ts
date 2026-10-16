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
  webpack(config) {
    config.resolve ??= {};
    config.resolve.alias ??= {};
    config.resolve.alias["next-intl/config"] = path.resolve(process.cwd(), "src/i18n/i18n.ts");
    return config;
  },
};

export default nextConfig;
