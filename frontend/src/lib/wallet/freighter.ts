/**
 * Freighter integration with full error handling.
 *
 * Freighter's API resolves with `{ error }` instead of throwing, reports a
 * locked wallet as an empty address, and happily signs on whatever network the
 * user has selected. This module turns each of those cases into a typed
 * {@link WalletError} so the UI can explain what happened and how to fix it.
 */

import * as freighterApi from "@stellar/freighter-api";

import { toWalletError, WalletError, withWalletTimeout } from "./errors";

/** Matches the network used by `stellarService`. */
export const NETWORK_PASSPHRASE = "Test SDF Network ; September 2015";
export const NETWORK_NAME = "testnet";

/** Status checks should be instant; anything slower means the extension is stuck. */
const STATUS_TIMEOUT_MS = 5_000;

export interface FreighterStatus {
  installed: boolean;
  /** The user has previously allowed this site. */
  allowed: boolean;
  /** Passphrase of the network currently selected in Freighter, if known. */
  networkPassphrase?: string;
  /** True when Freighter's network matches the app's. */
  onExpectedNetwork: boolean;
}

/** Throws the `{ error }` field of a Freighter response as a WalletError. */
function assertNoError(
  response: { error?: { code: number; message: string } },
  action: "connect" | "sign",
): void {
  if (response.error) throw toWalletError("freighter", response.error, action);
}

export async function isFreighterInstalled(): Promise<boolean> {
  try {
    const { isConnected, error } = await withWalletTimeout(
      freighterApi.isConnected(),
      "freighter",
      STATUS_TIMEOUT_MS,
    );
    return !error && Boolean(isConnected);
  } catch {
    return false;
  }
}

/**
 * Reads Freighter's state without prompting the user. Never throws — a missing
 * or unresponsive extension reports as not installed.
 */
export async function getFreighterStatus(): Promise<FreighterStatus> {
  const installed = await isFreighterInstalled();
  if (!installed) {
    return { installed: false, allowed: false, onExpectedNetwork: false };
  }

  try {
    const [allowedResult, networkResult] = await Promise.all([
      freighterApi.isAllowed(),
      freighterApi.getNetworkDetails(),
    ]);
    const networkPassphrase = networkResult.error
      ? undefined
      : networkResult.networkPassphrase;

    return {
      installed: true,
      allowed: !allowedResult.error && Boolean(allowedResult.isAllowed),
      networkPassphrase,
      onExpectedNetwork: networkPassphrase === NETWORK_PASSPHRASE,
    };
  } catch {
    return { installed: true, allowed: false, onExpectedNetwork: false };
  }
}

/** Rejects with `wrong-network` unless Freighter is on the app's network. */
export async function assertFreighterNetwork(): Promise<void> {
  const details = await freighterApi.getNetworkDetails();
  // Older Freighter builds can't report the network; let the signing step
  // enforce the passphrase instead of blocking the user.
  if (details.error || !details.networkPassphrase) return;

  if (details.networkPassphrase !== NETWORK_PASSPHRASE) {
    throw new WalletError(
      "wrong-network",
      "freighter",
      `Freighter is on ${details.network || "another network"}, but kor-AssetForge uses Testnet.`,
    );
  }
}

/**
 * Prompts for access (and unlock, if needed) and returns the active account,
 * after checking that Freighter is installed and on the right network.
 */
export async function connectFreighter(): Promise<string> {
  if (!(await isFreighterInstalled())) {
    throw new WalletError(
      "not-installed",
      "freighter",
      "Freighter extension is not installed.",
    );
  }

  try {
    // requestAccess shows Freighter's unlock screen when the wallet is locked
    // and its permission prompt the first time a site connects.
    const response = await freighterApi.requestAccess();
    assertNoError(response, "connect");

    if (!response.address) {
      // An empty address without an error means Freighter stayed locked or
      // the user closed the popup before choosing an account.
      const { allowed } = await getFreighterStatus();
      throw new WalletError(
        allowed ? "locked" : "no-account",
        "freighter",
        allowed
          ? "Freighter is locked. Unlock it and try again."
          : "Freighter didn't share an account.",
      );
    }

    await assertFreighterNetwork();
    return response.address;
  } catch (error) {
    throw toWalletError("freighter", error, "connect");
  }
}

/**
 * Restores a session without prompting. Resolves to `null` whenever the user
 * would have to interact with Freighter (locked, not allowed, wrong network).
 */
export async function getFreighterAddressSilently(): Promise<string | null> {
  const status = await getFreighterStatus();
  if (!status.installed || !status.allowed || !status.onExpectedNetwork) {
    return null;
  }

  try {
    const { address, error } = await freighterApi.getAddress();
    return error || !address ? null : address;
  } catch {
    return null;
  }
}

/** Signs an envelope, distinguishing rejection, wrong network and failures. */
export async function signWithFreighter(
  xdr: string,
  publicKey: string,
): Promise<string> {
  try {
    await assertFreighterNetwork();

    const response = await freighterApi.signTransaction(xdr, {
      networkPassphrase: NETWORK_PASSPHRASE,
      address: publicKey,
    });
    assertNoError(response, "sign");

    if (!response.signedTxXdr) {
      throw new WalletError(
        "transaction-rejected",
        "freighter",
        "The transaction was not signed.",
      );
    }
    if (response.signerAddress && response.signerAddress !== publicKey) {
      throw new WalletError(
        "signing-failed",
        "freighter",
        "Freighter signed with a different account than the one connected. Switch accounts in Freighter or reconnect.",
      );
    }
    return response.signedTxXdr;
  } catch (error) {
    throw toWalletError("freighter", error, "sign");
  }
}

/**
 * Explains why a previously connected Freighter session can't be restored
 * silently, or `null` when there is nothing the user needs to fix.
 */
export async function diagnoseFreighter(): Promise<WalletError | null> {
  const status = await getFreighterStatus();
  // Not installed / never allowed: the normal connect flow covers these.
  if (!status.installed || !status.allowed) return null;

  if (status.networkPassphrase && !status.onExpectedNetwork) {
    return new WalletError(
      "wrong-network",
      "freighter",
      "Freighter is on a different network than kor-AssetForge.",
    );
  }

  try {
    const { address, error } = await freighterApi.getAddress();
    if (!error && !address) {
      return new WalletError("locked", "freighter", "Freighter is locked.");
    }
  } catch {
    // Treat an unreadable state as nothing to report.
  }
  return null;
}
