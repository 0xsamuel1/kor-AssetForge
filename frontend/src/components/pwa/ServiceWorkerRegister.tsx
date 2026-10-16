"use client";

import { useEffect } from "react";

import { toast } from "@/lib/toast";

/**
 * Registers `/sw.js` in production builds and offers a reload when a new
 * version has been deployed.
 *
 * Registration is skipped in development: a caching worker on `next dev`
 * serves stale chunks and fights hot reload.
 */
export function ServiceWorkerRegister() {
  useEffect(() => {
    if (process.env.NODE_ENV !== "production") return;
    if (typeof window === "undefined" || !("serviceWorker" in navigator)) return;

    let cancelled = false;
    let reloading = false;

    const onControllerChange = () => {
      if (reloading) return;
      reloading = true;
      window.location.reload();
    };

    const promptUpdate = (worker: ServiceWorker) => {
      toast.info("A new version of kor-AssetForge is available", {
        description: "Reload to get the latest features and fixes.",
        duration: 0,
        action: {
          label: "Reload",
          onClick: () => {
            navigator.serviceWorker.addEventListener(
              "controllerchange",
              onControllerChange,
            );
            worker.postMessage({ type: "SKIP_WAITING" });
          },
        },
      });
    };

    const register = async () => {
      try {
        const registration = await navigator.serviceWorker.register("/sw.js", {
          scope: "/",
          updateViaCache: "none",
        });
        if (cancelled) return;

        // An update finished installing while the tab was closed.
        if (registration.waiting && navigator.serviceWorker.controller) {
          promptUpdate(registration.waiting);
        }

        registration.addEventListener("updatefound", () => {
          const installing = registration.installing;
          if (!installing) return;
          installing.addEventListener("statechange", () => {
            // Only prompt when replacing an existing worker, not on first install.
            if (
              installing.state === "installed" &&
              navigator.serviceWorker.controller
            ) {
              promptUpdate(installing);
            }
          });
        });
      } catch (error) {
        // Offline support is progressive: the app still works without it.
        console.warn("Service worker registration failed:", error);
      }
    };

    if (document.readyState === "complete") {
      register();
    } else {
      window.addEventListener("load", register, { once: true });
    }

    return () => {
      cancelled = true;
      window.removeEventListener("load", register);
    };
  }, []);

  return null;
}
