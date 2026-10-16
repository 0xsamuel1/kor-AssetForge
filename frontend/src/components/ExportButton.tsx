"use client";

import React, { useState } from "react";
import { generatePDF, ReportType, BrandingOptions, PortfolioRow, TransactionRow, TaxRow } from "@/lib/pdf-generator";

interface ExportButtonProps {
  reportType: ReportType;
  title?: string;
  subtitle?: string;
  branding?: BrandingOptions;
  chartImageDataUrl?: string;
  portfolioRows?: PortfolioRow[];
  transactionRows?: TransactionRow[];
  taxRows?: TaxRow[];
  /** Button label override */
  label?: string;
  className?: string;
  disabled?: boolean;
}

export const ExportButton: React.FC<ExportButtonProps> = ({
  reportType,
  title,
  subtitle,
  branding,
  chartImageDataUrl,
  portfolioRows,
  transactionRows,
  taxRows,
  label = "Export PDF",
  className = "",
  disabled = false,
}) => {
  const [progress, setProgress] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);

  const handleExport = async () => {
    setError(null);
    setProgress(0);

    try {
      await generatePDF({
        type: reportType,
        title,
        subtitle,
        branding,
        chartImageDataUrl,
        portfolioRows,
        transactionRows,
        taxRows,
        onProgress: setProgress,
      });
    } catch (err) {
      setError("Export failed. Please try again.");
      console.error("[ExportButton]", err);
    } finally {
      // Brief pause so user sees 100%
      setTimeout(() => setProgress(null), 800);
    }
  };

  const isLoading = progress !== null;

  return (
    <div className={`inline-flex flex-col items-end gap-1.5 ${className}`}>
      <button
        onClick={handleExport}
        disabled={disabled || isLoading}
        aria-busy={isLoading}
        className={[
          "inline-flex items-center gap-2 px-4 py-2 rounded-lg text-sm font-medium transition-all",
          "bg-indigo-600 text-white hover:bg-indigo-700 active:bg-indigo-800",
          "disabled:opacity-60 disabled:cursor-not-allowed",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-indigo-500 focus-visible:ring-offset-2",
        ].join(" ")}
      >
        {isLoading ? (
          <>
            <svg
              className="animate-spin w-4 h-4"
              viewBox="0 0 24 24"
              fill="none"
              aria-hidden="true"
            >
              <circle
                className="opacity-25"
                cx="12"
                cy="12"
                r="10"
                stroke="currentColor"
                strokeWidth="4"
              />
              <path
                className="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8v4l3-3-3-3v4a8 8 0 00-8 8h4z"
              />
            </svg>
            {progress !== null && progress < 100
              ? `Generating… ${progress}%`
              : "Saving…"}
          </>
        ) : (
          <>
            <svg
              className="w-4 h-4"
              viewBox="0 0 20 20"
              fill="currentColor"
              aria-hidden="true"
            >
              <path
                fillRule="evenodd"
                d="M3 17a1 1 0 011-1h12a1 1 0 110 2H4a1 1 0 01-1-1zm3.293-7.707a1 1 0 011.414 0L9 10.586V3a1 1 0 112 0v7.586l1.293-1.293a1 1 0 111.414 1.414l-3 3a1 1 0 01-1.414 0l-3-3a1 1 0 010-1.414z"
                clipRule="evenodd"
              />
            </svg>
            {label}
          </>
        )}
      </button>

      {/* Progress bar */}
      {isLoading && (
        <div
          className="h-1 w-full rounded-full bg-indigo-100 overflow-hidden"
          role="progressbar"
          aria-valuenow={progress ?? 0}
          aria-valuemin={0}
          aria-valuemax={100}
        >
          <div
            className="h-full bg-indigo-500 transition-all duration-200"
            style={{ width: `${progress ?? 0}%` }}
          />
        </div>
      )}

      {/* Error state */}
      {error && (
        <p className="text-xs text-red-500" role="alert">
          {error}
        </p>
      )}
    </div>
  );
};

export default ExportButton;
