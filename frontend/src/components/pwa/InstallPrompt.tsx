"use client";

import { useCallback, useEffect, useState, useSyncExternalStore } from "react";
import { Download, Share, X } from "lucide-react";

import { Button } from "@/components/ui/button";

/** Chromium's install event — not in the DOM lib typings yet. */
interface BeforeInstallPromptEvent extends Event {
  prompt: () => Promise<void>;
  userChoice: Promise<{ outcome: "accepted" | "dismissed"; platform: string }>;
}

const DISMISSED_KEY = "kor-assetforge:install-dismissed-at";
/** After "Not now", stay quiet for two weeks. */
const DISMISS_FOR_MS = 14 * 24 * 60 * 60 * 1000;

function wasRecentlyDismissed(): boolean {
  try {
    const at = Number(window.localStorage.getItem(DISMISSED_KEY));
    return Number.isFinite(at) && at > 0 && Date.now() - at < DISMISS_FOR_MS;
  } catch {
    return false;
  }
}

function rememberDismissal() {
  try {
    window.localStorage.setItem(DISMISSED_KEY, String(Date.now()));
  } catch {
    // Storage may be blocked — the prompt simply comes back next visit.
  }
}

function isStandalone(): boolean {
  return (
    window.matchMedia("(display-mode: standalone)").matches ||
    // iOS Safari
    (navigator as Navigator & { standalone?: boolean }).standalone === true
  );
}

/** iOS has no install event, so Safari users get manual instructions. */
function shouldShowIosHint(): boolean {
  const ua = navigator.userAgent;
  const isIos =
    /iphone|ipad|ipod/i.test(ua) ||
    // iPadOS reports itself as macOS
    (/macintosh/i.test(ua) && navigator.maxTouchPoints > 1);
  return isIos && !isStandalone() && !wasRecentlyDismissed();
}

const noopSubscribe = () => () => {};

/**
 * "Install app" card. Uses the native prompt where the browser supports it
 * (Chrome, Edge, Android) and shows Add-to-Home-Screen steps on iOS.
 */
export function InstallPrompt() {
  const [deferred, setDeferred] = useState<BeforeInstallPromptEvent | null>(null);
  const [isHidden, setIsHidden] = useState(false);
  const showIosHint = useSyncExternalStore(
    noopSubscribe,
    shouldShowIosHint,
    () => false,
  );

  useEffect(() => {
    const onBeforeInstall = (event: Event) => {
      // Hold the event so the prompt opens from our button instead.
      event.preventDefault();
      if (isStandalone() || wasRecentlyDismissed()) return;
      setDeferred(event as BeforeInstallPromptEvent);
    };
    const onInstalled = () => setDeferred(null);

    window.addEventListener("beforeinstallprompt", onBeforeInstall);
    window.addEventListener("appinstalled", onInstalled);
    return () => {
      window.removeEventListener("beforeinstallprompt", onBeforeInstall);
      window.removeEventListener("appinstalled", onInstalled);
    };
  }, []);

  const dismiss = useCallback(() => {
    rememberDismissal();
    setIsHidden(true);
  }, []);

  const install = useCallback(async () => {
    if (!deferred) return;
    await deferred.prompt();
    const { outcome } = await deferred.userChoice;
    if (outcome === "dismissed") rememberDismissal();
    // A prompt event can only be used once.
    setDeferred(null);
  }, [deferred]);

  if (isHidden || (!deferred && !showIosHint)) return null;

  return (
    <div
      role="dialog"
      aria-labelledby="install-prompt-title"
      aria-describedby="install-prompt-description"
      className="fixed bottom-4 right-4 z-50 w-[calc(100%-2rem)] max-w-sm rounded-xl bg-card p-4 text-card-foreground shadow-lg ring-1 ring-foreground/10"
    >
      <div className="flex items-start gap-3">
        <div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground">
          <Download className="size-5" aria-hidden="true" />
        </div>
        <div className="min-w-0 flex-1">
          <p id="install-prompt-title" className="text-sm font-semibold">
            Install kor-AssetForge
          </p>
          <p
            id="install-prompt-description"
            className="mt-1 text-xs text-muted-foreground"
          >
            {deferred ? (
              "Add the app to your device for faster access and basic offline support."
            ) : (
              <>
                Tap <Share className="inline size-3.5 align-text-bottom" aria-label="Share" />{" "}
                then <strong>Add to Home Screen</strong>.
              </>
            )}
          </p>
          {deferred && (
            <div className="mt-3 flex gap-2">
              <Button size="sm" onClick={install}>
                Install
              </Button>
              <Button size="sm" variant="ghost" onClick={dismiss}>
                Not now
              </Button>
            </div>
          )}
        </div>
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={dismiss}
          aria-label="Dismiss install prompt"
        >
          <X className="size-4" aria-hidden="true" />
        </Button>
      </div>
    </div>
  );
}
