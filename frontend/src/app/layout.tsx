import type { Metadata, Viewport } from "next";
import { Geist, Geist_Mono } from "next/font/google";
import "./globals.css";
import "@/styles/forms.css";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import { ToastContainer } from "@/components/ui/Toast";
import { OnboardingProvider } from "@/components/onboarding/OnboardingProvider";
import { ServiceWorkerRegister } from "@/components/pwa/ServiceWorkerRegister";
import { InstallPrompt } from "@/components/pwa/InstallPrompt";
import { OfflineIndicator } from "@/components/pwa/OfflineIndicator";

const geistSans = Geist({
  variable: "--font-geist-sans",
  subsets: ["latin"],
});

const geistMono = Geist_Mono({
  variable: "--font-geist-mono",
  subsets: ["latin"],
});

export const metadata: Metadata = {
  title: "kor-AssetForge",
  description: "Decentralized marketplace for tokenizing and trading real-world assets on Stellar",
  applicationName: "kor-AssetForge",
  appleWebApp: {
    capable: true,
    title: "AssetForge",
    statusBarStyle: "default",
  },
  icons: {
    apple: "/apple-touch-icon.png",
  },
};

export const viewport: Viewport = {
  themeColor: [
    { media: "(prefers-color-scheme: light)", color: "#ffffff" },
    { media: "(prefers-color-scheme: dark)", color: "#0a0a0a" },
  ],
};

import { NextIntlClientProvider } from 'next-intl';
import { getLocale, getMessages } from 'next-intl/server';

export default async function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  const locale = await getLocale();
  const messages = await getMessages();

  return (
    <html
      lang={locale}
      className={`${geistSans.variable} ${geistMono.variable} h-full antialiased`}
    >
      <body className="min-h-full flex flex-col">
        <a href="#main-content" className="skip-link">
          Skip to main content
        </a>
        <NextIntlClientProvider messages={messages} locale={locale}>
          <ErrorBoundary>
            <OnboardingProvider>
              {children}
            </OnboardingProvider>
          </ErrorBoundary>
        </NextIntlClientProvider>
        <ToastContainer />
        <OfflineIndicator />
        <InstallPrompt />
        <ServiceWorkerRegister />
      </body>
    </html>
  );
}
