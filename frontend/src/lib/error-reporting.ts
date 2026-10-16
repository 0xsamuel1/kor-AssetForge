export interface ErrorReport {
  message: string
  stack?: string
  componentStack?: string
  timestamp: number
  url: string
  userAgent: string
}

type ErrorHandler = (error: ErrorReport) => void

let handlers: ErrorHandler[] = []

const isDev = process.env.NODE_ENV === "development"

export function addErrorHandler(handler: ErrorHandler): () => void {
  handlers.push(handler)
  return () => {
    handlers = handlers.filter((h) => h !== handler)
  }
}

export function reportError(
  error: Error | unknown,
  componentStack?: string,
): void {
  const err = error instanceof Error ? error : new Error(String(error))

  const report: ErrorReport = {
    message: err.message,
    stack: isDev ? err.stack : undefined,
    componentStack,
    timestamp: Date.now(),
    url: typeof window !== "undefined" ? window.location.href : "",
    userAgent: typeof navigator !== "undefined" ? navigator.userAgent : "",
  }

  if (isDev) {
    console.error("[ErrorReport]", report)
  }

  handlers.forEach((handler) => handler(report))

  sendErrorToBackend(report)
}

async function sendErrorToBackend(report: ErrorReport): Promise<void> {
  try {
    const backendUrl =
      process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080"
    await fetch(`${backendUrl}/api/v1/errors`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(report),
    })
  } catch {
    console.warn("Failed to report error to backend")
  }
}

export function clearErrorHandlers(): void {
  handlers = []
}
