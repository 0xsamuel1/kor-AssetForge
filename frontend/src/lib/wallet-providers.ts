/**
 * Stellar wallet provider registry.
 *
 * Each provider is described once here — detection, connect, sign and
 * disconnect — so the UI (`WalletModal`, `WalletConnect`) never has to know how
 * an individual wallet exposes itself to the page.
 *
 * Freighter talks through its official SDK; the others inject a global object
 * when their extension (or, for Albedo, their intent script) is present, which
 * is what `isAvailable()` probes for.
 */

import type { StellarWallet } from "@/lib/stellar";
import {
  toWalletError,
  WalletError,
  type WalletProviderId,
} from "@/lib/wallet/errors";
import {
  connectFreighter,
  getFreighterAddressSilently,
  isFreighterInstalled,
  NETWORK_NAME,
  NETWORK_PASSPHRASE,
  signWithFreighter,
} from "@/lib/wallet/freighter";

// Re-exported so existing imports from this module keep working.
export { NETWORK_NAME, NETWORK_PASSPHRASE, WalletError };
export type { WalletErrorCode, WalletProviderId } from "@/lib/wallet/errors";

const STORAGE_KEY = "kor-assetforge:wallet-provider";

export interface WalletProvider {
  id: WalletProviderId;
  name: string;
  description: string;
  /** Where to get the wallet when it isn't detected. */
  installUrl: string;
  /** Initials shown in the provider tile, so no remote images are needed. */
  initials: string;
  /** Tailwind classes for the tile badge. */
  badgeClassName: string;
  /** Whether the wallet lives in the browser or opens a hosted popup. */
  kind: "extension" | "web";
  /** True when the wallet can be used from this browser right now. */
  isAvailable: () => Promise<boolean>;
  /** Requests access and resolves the account's public key. */
  connect: () => Promise<string>;
  /** Signs a transaction envelope, returning the signed XDR. */
  signTransaction: (xdr: string, publicKey: string) => Promise<string>;
  /** Optional provider-side cleanup; local state is always cleared regardless. */
  disconnect?: () => Promise<void>;
}

export interface ConnectedWallet extends StellarWallet {
  providerId: WalletProviderId;
  providerName: string;
}

// ── Injected globals ─────────────────────────────────────────────────────────
// Minimal structural types for the objects each wallet puts on `window`.

interface AlbedoApi {
  publicKey: (params: { token?: string }) => Promise<{ pubkey: string }>;
  tx: (params: {
    xdr: string;
    network?: string;
    pubkey?: string;
  }) => Promise<{ signed_envelope_xdr: string }>;
}

interface LobstrApi {
  isConnected: () => Promise<boolean>;
  getPublicKey: () => Promise<string>;
  signTransaction?: (xdr: string) => Promise<string>;
}

interface XBullApi {
  connect: (permissions: {
    canRequestPublicKey: boolean;
    canRequestSign: boolean;
  }) => Promise<unknown>;
  getPublicKey: () => Promise<string>;
  signXDR: (
    xdr: string,
    options?: { publicKey?: string; network?: string; networkPassphrase?: string },
  ) => Promise<string>;
  closeConnection?: () => Promise<void>;
}

interface RabetApi {
  connect: () => Promise<{ publicKey: string }>;
  sign: (xdr: string, network: string) => Promise<{ xdr: string }>;
  disconnect: () => Promise<void>;
}

declare global {
  interface Window {
    albedo?: AlbedoApi;
    lobstrExtension?: LobstrApi;
    xBullSDK?: XBullApi;
    rabet?: RabetApi;
  }
}

/** Extensions inject asynchronously, so give a late-loading global a moment. */
function waitForGlobal<T>(read: () => T | undefined, timeout = 600): Promise<T | undefined> {
  if (typeof window === "undefined") return Promise.resolve(undefined);

  const immediate = read();
  if (immediate) return Promise.resolve(immediate);

  return new Promise((resolve) => {
    const startedAt = Date.now();
    const poll = window.setInterval(() => {
      const value = read();
      if (value || Date.now() - startedAt >= timeout) {
        window.clearInterval(poll);
        resolve(value);
      }
    }, 100);
  });
}

// ── Providers ────────────────────────────────────────────────────────────────

const freighter: WalletProvider = {
  id: "freighter",
  name: "Freighter",
  description: "Browser extension by the Stellar Development Foundation",
  installUrl: "https://www.freighter.app/",
  initials: "FR",
  badgeClassName: "bg-indigo-500/15 text-indigo-600 dark:text-indigo-400",
  kind: "extension",

  isAvailable: isFreighterInstalled,
  connect: connectFreighter,
  signTransaction: signWithFreighter,
};

const albedo: WalletProvider = {
  id: "albedo",
  name: "Albedo",
  description: "Web wallet — signs in a popup, nothing to install",
  installUrl: "https://albedo.link/",
  initials: "AL",
  badgeClassName: "bg-amber-500/15 text-amber-600 dark:text-amber-400",
  kind: "web",

  async isAvailable() {
    return Boolean(await waitForGlobal(() => window.albedo));
  },

  async connect() {
    const api = await waitForGlobal(() => window.albedo);
    if (!api) {
      throw new WalletError(
        "not-installed",
        "albedo",
        "Albedo is not available on this page.",
      );
    }

    try {
      const { pubkey } = await api.publicKey({});
      return pubkey;
    } catch (error) {
      throw toWalletError("albedo", error, "connect");
    }
  },

  async signTransaction(xdr, publicKey) {
    const api = window.albedo;
    if (!api) {
      throw new WalletError("not-installed", "albedo", "Albedo is not available.");
    }

    try {
      const { signed_envelope_xdr } = await api.tx({
        xdr,
        network: NETWORK_NAME,
        pubkey: publicKey,
      });
      return signed_envelope_xdr;
    } catch (error) {
      throw toWalletError("albedo", error, "sign");
    }
  },
};

const lobstr: WalletProvider = {
  id: "lobstr",
  name: "LOBSTR",
  description: "LOBSTR signer extension",
  installUrl: "https://lobstr.co/",
  initials: "LO",
  badgeClassName: "bg-sky-500/15 text-sky-600 dark:text-sky-400",
  kind: "extension",

  async isAvailable() {
    const api = await waitForGlobal(() => window.lobstrExtension);
    if (!api) return false;

    try {
      // Present but locked still counts as available — connect() will prompt.
      await api.isConnected();
      return true;
    } catch {
      return false;
    }
  },

  async connect() {
    const api = await waitForGlobal(() => window.lobstrExtension);
    if (!api) {
      throw new WalletError(
        "not-installed",
        "lobstr",
        "LOBSTR signer extension is not installed.",
      );
    }

    try {
      return await api.getPublicKey();
    } catch (error) {
      throw toWalletError("lobstr", error, "connect");
    }
  },

  async signTransaction(xdr) {
    const api = window.lobstrExtension;
    if (!api?.signTransaction) {
      throw new WalletError(
        "unsupported",
        "lobstr",
        "This LOBSTR version cannot sign transactions from the browser.",
      );
    }

    try {
      return await api.signTransaction(xdr);
    } catch (error) {
      throw toWalletError("lobstr", error, "sign");
    }
  },
};

const xbull: WalletProvider = {
  id: "xbull",
  name: "xBull",
  description: "Open-source Stellar wallet extension",
  installUrl: "https://xbull.app/",
  initials: "XB",
  badgeClassName: "bg-emerald-500/15 text-emerald-600 dark:text-emerald-400",
  kind: "extension",

  async isAvailable() {
    return Boolean(await waitForGlobal(() => window.xBullSDK));
  },

  async connect() {
    const api = await waitForGlobal(() => window.xBullSDK);
    if (!api) {
      throw new WalletError(
        "not-installed",
        "xbull",
        "xBull extension is not installed.",
      );
    }

    try {
      await api.connect({ canRequestPublicKey: true, canRequestSign: true });
      return await api.getPublicKey();
    } catch (error) {
      throw toWalletError("xbull", error, "connect");
    }
  },

  async signTransaction(xdr, publicKey) {
    const api = window.xBullSDK;
    if (!api) {
      throw new WalletError("not-installed", "xbull", "xBull is not available.");
    }

    try {
      return await api.signXDR(xdr, {
        publicKey,
        network: NETWORK_NAME,
        networkPassphrase: NETWORK_PASSPHRASE,
      });
    } catch (error) {
      throw toWalletError("xbull", error, "sign");
    }
  },

  async disconnect() {
    await window.xBullSDK?.closeConnection?.();
  },
};

const rabet: WalletProvider = {
  id: "rabet",
  name: "Rabet",
  description: "Lightweight Stellar wallet extension",
  installUrl: "https://rabet.io/",
  initials: "RB",
  badgeClassName: "bg-rose-500/15 text-rose-600 dark:text-rose-400",
  kind: "extension",

  async isAvailable() {
    return Boolean(await waitForGlobal(() => window.rabet));
  },

  async connect() {
    const api = await waitForGlobal(() => window.rabet);
    if (!api) {
      throw new WalletError(
        "not-installed",
        "rabet",
        "Rabet extension is not installed.",
      );
    }

    try {
      const { publicKey } = await api.connect();
      return publicKey;
    } catch (error) {
      throw toWalletError("rabet", error, "connect");
    }
  },

  async signTransaction(xdr) {
    const api = window.rabet;
    if (!api) {
      throw new WalletError("not-installed", "rabet", "Rabet is not available.");
    }

    try {
      const result = await api.sign(xdr, NETWORK_NAME);
      return result.xdr;
    } catch (error) {
      throw toWalletError("rabet", error, "sign");
    }
  },

  async disconnect() {
    await window.rabet?.disconnect();
  },
};

/** Every provider the app supports, in the order they appear in the modal. */
export const WALLET_PROVIDERS: readonly WalletProvider[] = [
  freighter,
  albedo,
  lobstr,
  xbull,
  rabet,
];

export function getWalletProvider(
  id: WalletProviderId,
): WalletProvider | undefined {
  return WALLET_PROVIDERS.find((provider) => provider.id === id);
}

/** Probes every provider in parallel — used to badge the modal tiles. */
export async function detectWalletProviders(): Promise<
  Record<WalletProviderId, boolean>
> {
  const entries = await Promise.all(
    WALLET_PROVIDERS.map(
      async (provider) =>
        [provider.id, await provider.isAvailable()] as const,
    ),
  );

  return Object.fromEntries(entries) as Record<WalletProviderId, boolean>;
}

// ── Session persistence ──────────────────────────────────────────────────────

/** The provider used last, so the app can offer a one-click reconnect. */
export function getStoredProviderId(): WalletProviderId | null {
  if (typeof window === "undefined") return null;

  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    return stored && getWalletProvider(stored as WalletProviderId)
      ? (stored as WalletProviderId)
      : null;
  } catch {
    // Storage can be unavailable (private mode, blocked cookies) — not fatal.
    return null;
  }
}

function storeProviderId(id: WalletProviderId): void {
  try {
    window.localStorage.setItem(STORAGE_KEY, id);
  } catch {
    // Ignore — persistence is a convenience, not a requirement.
  }
}

function clearStoredProviderId(): void {
  try {
    window.localStorage.removeItem(STORAGE_KEY);
  } catch {
    // Ignore.
  }
}

// ── Public actions ───────────────────────────────────────────────────────────

/** Connects through a specific provider and remembers the choice. */
export async function connectWallet(
  id: WalletProviderId,
): Promise<ConnectedWallet> {
  const provider = getWalletProvider(id);
  if (!provider) {
    throw new WalletError("unknown", id, `Unknown wallet provider: ${id}`);
  }

  const publicKey = await provider.connect();
  storeProviderId(id);

  return {
    publicKey,
    connected: true,
    providerId: provider.id,
    providerName: provider.name,
  };
}

/** Disconnects the provider (where supported) and forgets the stored choice. */
export async function disconnectWallet(
  id: WalletProviderId | null,
): Promise<void> {
  if (id) {
    try {
      await getWalletProvider(id)?.disconnect?.();
    } catch {
      // A failed provider-side disconnect must not block local cleanup.
    }
  }
  clearStoredProviderId();
}

/** Signs an envelope with the connected wallet. */
export async function signTransactionWithWallet(
  wallet: ConnectedWallet,
  xdr: string,
): Promise<string> {
  const provider = getWalletProvider(wallet.providerId);
  if (!provider) {
    throw new WalletError(
      "unknown",
      wallet.providerId,
      "The connected wallet is no longer available.",
    );
  }
  return provider.signTransaction(xdr, wallet.publicKey);
}

/**
 * Silently restores the previous session on page load. Resolves to `null` when
 * there is nothing to restore or the wallet would need a fresh prompt.
 */
export async function restoreWalletConnection(): Promise<ConnectedWallet | null> {
  const id = getStoredProviderId();
  if (!id) return null;

  const provider = getWalletProvider(id);
  if (!provider || !(await provider.isAvailable())) return null;

  try {
    // Freighter can be queried without a popup; it returns null when locked,
    // not yet allowed, or on the wrong network so the modal can explain why.
    const publicKey =
      id === "freighter"
        ? await getFreighterAddressSilently()
        : await provider.connect();
    if (!publicKey) return null;
    return {
      publicKey,
      connected: true,
      providerId: provider.id,
      providerName: provider.name,
    };
  } catch {
    // The user has to re-authorise — surface the modal instead of an error.
    return null;
  }
}
