/**
 * Wallet error definitions.
 *
 * Every failure from a wallet provider is normalised into a {@link WalletError}
 * with a stable `code`, so the UI can show a clear title, an explanation and
 * concrete recovery steps instead of a raw extension message.
 */

export type WalletProviderId =
  | "freighter"
  | "albedo"
  | "lobstr"
  | "xbull"
  | "rabet";

export type WalletErrorCode =
  /** The extension isn't installed (or the page can't see it). */
  | "not-installed"
  /** The extension is installed but locked behind its password. */
  | "locked"
  /** The user closed or declined the connection prompt. */
  | "rejected"
  /** The user declined to sign a transaction. */
  | "transaction-rejected"
  /** The wallet is on a different Stellar network than the app. */
  | "wrong-network"
  /** The wallet has no account, or didn't share one. */
  | "no-account"
  /** The wallet didn't answer in time. */
  | "timeout"
  /** The wallet can't perform this action. */
  | "unsupported"
  /** The wallet accepted the request but failed while signing. */
  | "signing-failed"
  | "unknown";

/** What the wallet was asked to do when it failed. */
export type WalletAction = "connect" | "sign";

/** Wallet failures we can explain to the user, rather than a raw exception. */
export class WalletError extends Error {
  readonly code: WalletErrorCode;
  readonly providerId: WalletProviderId;
  /** The provider's original error, kept for logging. */
  readonly cause?: unknown;

  constructor(
    code: WalletErrorCode,
    providerId: WalletProviderId,
    message: string,
    cause?: unknown,
  ) {
    super(message);
    this.name = "WalletError";
    this.code = code;
    this.providerId = providerId;
    this.cause = cause;
  }
}

export function isWalletError(error: unknown): error is WalletError {
  return error instanceof WalletError;
}

export interface WalletErrorDetails {
  code: WalletErrorCode;
  title: string;
  message: string;
  /** Ordered steps the user can take to fix the problem. */
  recovery: string[];
  /** Whether simply trying again can succeed without the user changing anything. */
  retryable: boolean;
}

const PROVIDER_NAMES: Record<WalletProviderId, string> = {
  freighter: "Freighter",
  albedo: "Albedo",
  lobstr: "LOBSTR",
  xbull: "xBull",
  rabet: "Rabet",
};

/** Name of the network the app expects, shown in wrong-network guidance. */
export const EXPECTED_NETWORK_LABEL = "Testnet";

type DetailsFactory = (wallet: string) => Omit<WalletErrorDetails, "code">;

const ERROR_DETAILS: Record<WalletErrorCode, DetailsFactory> = {
  "not-installed": (wallet) => ({
    title: `${wallet} not found`,
    message: `We couldn't detect the ${wallet} wallet in this browser.`,
    recovery: [
      `Install the ${wallet} extension from its official website.`,
      "Make sure the extension is enabled for this site.",
      "Reload this page after installing, then try again.",
    ],
    retryable: false,
  }),
  locked: (wallet) => ({
    title: `${wallet} is locked`,
    message: `Your ${wallet} wallet needs to be unlocked before it can connect.`,
    recovery: [
      `Open the ${wallet} extension from your browser toolbar.`,
      "Enter your password to unlock it.",
      "Come back here and select Try again.",
    ],
    retryable: true,
  }),
  rejected: (wallet) => ({
    title: "Connection request declined",
    message: `The connection request was closed or declined in ${wallet}.`,
    recovery: [
      "Select Try again to send a new request.",
      `Approve the request in the ${wallet} popup to share your public key.`,
    ],
    retryable: true,
  }),
  "transaction-rejected": (wallet) => ({
    title: "Transaction rejected",
    message: `You declined to sign the transaction in ${wallet}. Nothing was submitted and no funds moved.`,
    recovery: [
      "Review the transaction details, then submit it again.",
      `Approve the signing request in ${wallet} to continue.`,
    ],
    retryable: true,
  }),
  "wrong-network": (wallet) => ({
    title: "Wrong network",
    message: `${wallet} is connected to a different Stellar network. kor-AssetForge runs on ${EXPECTED_NETWORK_LABEL}.`,
    recovery: [
      `Open ${wallet} and go to its network settings.`,
      `Switch the network to ${EXPECTED_NETWORK_LABEL}.`,
      "Come back here and select Try again.",
    ],
    retryable: true,
  }),
  "no-account": (wallet) => ({
    title: "No account available",
    message: `${wallet} didn't share an account with this site.`,
    recovery: [
      `Create or import an account in ${wallet} if you haven't yet.`,
      `Make sure kor-AssetForge is allowed in ${wallet}'s connected sites.`,
      "Select Try again.",
    ],
    retryable: true,
  }),
  timeout: (wallet) => ({
    title: `${wallet} is not responding`,
    message: `${wallet} didn't respond in time.`,
    recovery: [
      `Check for a hidden ${wallet} popup or open the extension directly.`,
      "Reload the page if the extension was just installed or updated.",
      "Select Try again.",
    ],
    retryable: true,
  }),
  unsupported: (wallet) => ({
    title: "Action not supported",
    message: `This version of ${wallet} can't perform this action from the browser.`,
    recovery: [
      `Update ${wallet} to the latest version.`,
      "Or connect with a different wallet.",
    ],
    retryable: false,
  }),
  "signing-failed": (wallet) => ({
    title: "Signing failed",
    message: `${wallet} couldn't sign the transaction.`,
    recovery: [
      `Check that the active ${wallet} account matches the connected address.`,
      `Confirm ${wallet} is on ${EXPECTED_NETWORK_LABEL}.`,
      "Try again, or reconnect your wallet.",
    ],
    retryable: true,
  }),
  unknown: (wallet) => ({
    title: "Wallet error",
    message: `Something went wrong while talking to ${wallet}.`,
    recovery: [
      "Select Try again.",
      `If it keeps failing, reload the page or restart ${wallet}.`,
    ],
    retryable: true,
  }),
};

/** User-facing title, explanation and recovery steps for a wallet error. */
export function getWalletErrorDetails(
  error: unknown,
  providerId: WalletProviderId = "freighter",
): WalletErrorDetails {
  const code = isWalletError(error) ? error.code : "unknown";
  const wallet =
    PROVIDER_NAMES[isWalletError(error) ? error.providerId : providerId] ??
    "your wallet";
  const details = ERROR_DETAILS[code](wallet);

  // Keep a provider's own explanation for errors we can't classify — it is
  // usually more useful than a generic line.
  const message =
    code === "unknown" && isWalletError(error) && error.message
      ? error.message
      : details.message;

  return { code, ...details, message };
}

// ── Classification ───────────────────────────────────────────────────────────

/** SEP-43 / Freighter error code for "the user rejected this request". */
const SEP43_USER_REJECTED = -4;

const LOCKED_PATTERN = /\block(ed)?\b|unlock|password/i;
const REJECTED_PATTERN = /reject|denied|deny|cancel|declin|dismiss|closed/i;
const NETWORK_PATTERN = /network|passphrase/i;
const NOT_INSTALLED_PATTERN = /not (installed|found|detected)|no wallet|extension/i;
const TIMEOUT_PATTERN = /time(d)? ?out/i;
const NO_ACCOUNT_PATTERN = /no (account|address|public key)|not allowed/i;

function readMessage(error: unknown): string {
  if (error instanceof Error) return error.message;
  if (typeof error === "string") return error;
  if (error && typeof error === "object" && "message" in error) {
    return String((error as { message: unknown }).message ?? "");
  }
  return "";
}

function readCode(error: unknown): number | undefined {
  if (error && typeof error === "object" && "code" in error) {
    const code = (error as { code: unknown }).code;
    return typeof code === "number" ? code : undefined;
  }
  return undefined;
}

/**
 * Turns anything a wallet throws or returns into a {@link WalletError}.
 * `action` decides whether a rejection means "declined to connect" or
 * "declined to sign".
 */
export function toWalletError(
  providerId: WalletProviderId,
  error: unknown,
  action: WalletAction = "connect",
): WalletError {
  if (isWalletError(error)) return error;

  const message = readMessage(error);
  const code = readCode(error);

  let walletCode: WalletErrorCode;
  if (code === SEP43_USER_REJECTED || REJECTED_PATTERN.test(message)) {
    walletCode = action === "sign" ? "transaction-rejected" : "rejected";
  } else if (LOCKED_PATTERN.test(message)) {
    walletCode = "locked";
  } else if (TIMEOUT_PATTERN.test(message)) {
    walletCode = "timeout";
  } else if (NO_ACCOUNT_PATTERN.test(message)) {
    walletCode = "no-account";
  } else if (NETWORK_PATTERN.test(message)) {
    walletCode = "wrong-network";
  } else if (NOT_INSTALLED_PATTERN.test(message)) {
    walletCode = "not-installed";
  } else {
    walletCode = action === "sign" ? "signing-failed" : "unknown";
  }

  const fallback = ERROR_DETAILS[walletCode](PROVIDER_NAMES[providerId]).message;
  return new WalletError(
    walletCode,
    providerId,
    walletCode === "unknown" && message ? message : fallback,
    error,
  );
}

/** Rejects with a `timeout` WalletError if `promise` doesn't settle in time. */
export function withWalletTimeout<T>(
  promise: Promise<T>,
  providerId: WalletProviderId,
  ms: number,
): Promise<T> {
  let timer: ReturnType<typeof setTimeout> | undefined;
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(
      () =>
        reject(
          new WalletError(
            "timeout",
            providerId,
            ERROR_DETAILS.timeout(PROVIDER_NAMES[providerId]).message,
          ),
        ),
      ms,
    );
  });
  return Promise.race([promise, timeout]).finally(() => clearTimeout(timer));
}
