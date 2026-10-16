"use client";

import { createContext, useContext, useState, useCallback, useEffect, type ReactNode } from "react";

type OnboardingStep = "welcome" | "features" | "wallet" | "kyc" | "completion";

interface OnboardingContextValue {
  isOpen: boolean
  currentStep: OnboardingStep
  stepIndex: number
  totalSteps: number
  hasCompletedOnboarding: boolean
  openOnboarding: () => void
  closeOnboarding: () => void
  nextStep: () => void
  prevStep: () => void
  skipOnboarding: () => void
  completeOnboarding: () => void
  goToStep: (step: OnboardingStep) => void
}

const STEPS: OnboardingStep[] = ["welcome", "features", "wallet", "kyc", "completion"];
const STORAGE_KEY = "kor-assetforge-onboarding-completed";

const OnboardingContext = createContext<OnboardingContextValue | null>(null);

export function useOnboarding() {
  const ctx = useContext(OnboardingContext);
  if (!ctx) throw new Error("useOnboarding must be used within OnboardingProvider");
  return ctx;
}

export function OnboardingProvider({ children, autoShow = true }: { children: ReactNode; autoShow?: boolean }) {
  const [isOpen, setIsOpen] = useState(false);
  const [currentStep, setCurrentStep] = useState<OnboardingStep>("welcome");
  const [hasCompletedOnboarding, setHasCompletedOnboarding] = useState(true);

  useEffect(() => {
    const completed = localStorage.getItem(STORAGE_KEY) === "true";
    setHasCompletedOnboarding(completed);
    if (autoShow && !completed) {
      const timer = setTimeout(() => setIsOpen(true), 600);
      return () => clearTimeout(timer);
    }
  }, [autoShow]);

  const stepIndex = STEPS.indexOf(currentStep);
  const totalSteps = STEPS.length;

  const openOnboarding = useCallback(() => {
    setIsOpen(true);
    setCurrentStep("welcome");
  }, []);

  const closeOnboarding = useCallback(() => setIsOpen(false), []);

  const nextStep = useCallback(() => {
    const nextIndex = stepIndex + 1;
    if (nextIndex < STEPS.length) {
      setCurrentStep(STEPS[nextIndex]);
    } else {
      completeOnboarding();
    }
  }, [stepIndex]);

  const prevStep = useCallback(() => {
    const prevIndex = stepIndex - 1;
    if (prevIndex >= 0) setCurrentStep(STEPS[prevIndex]);
  }, [stepIndex]);

  const skipOnboarding = useCallback(() => {
    localStorage.setItem(STORAGE_KEY, "true");
    setHasCompletedOnboarding(true);
    setIsOpen(false);
  }, []);

  const completeOnboarding = useCallback(() => {
    localStorage.setItem(STORAGE_KEY, "true");
    setHasCompletedOnboarding(true);
    setIsOpen(false);
  }, []);

  const goToStep = useCallback((step: OnboardingStep) => {
    setCurrentStep(step);
  }, []);

  return (
    <OnboardingContext.Provider
      value={{
        isOpen,
        currentStep,
        stepIndex,
        totalSteps,
        hasCompletedOnboarding,
        openOnboarding,
        closeOnboarding,
        nextStep,
        prevStep,
        skipOnboarding,
        completeOnboarding,
        goToStep,
      }}
    >
      {children}
    </OnboardingContext.Provider>
  );
}
