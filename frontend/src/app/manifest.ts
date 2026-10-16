import type { MetadataRoute } from "next";

// Served at /manifest.webmanifest and linked from <head> automatically.
export default function manifest(): MetadataRoute.Manifest {
  return {
    id: "/",
    name: "kor-AssetForge",
    short_name: "AssetForge",
    description:
      "Decentralized marketplace for tokenizing and trading real-world assets on Stellar",
    start_url: "/",
    scope: "/",
    display: "standalone",
    orientation: "portrait-primary",
    background_color: "#ffffff",
    theme_color: "#171717",
    categories: ["finance", "business"],
    icons: [
      { src: "/icon-192.png", sizes: "192x192", type: "image/png", purpose: "any" },
      { src: "/icon-512.png", sizes: "512x512", type: "image/png", purpose: "any" },
      {
        src: "/icon-maskable-512.png",
        sizes: "512x512",
        type: "image/png",
        purpose: "maskable",
      },
    ],
  };
}
