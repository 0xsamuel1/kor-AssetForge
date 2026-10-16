"use client";

import { useOnboarding } from "./OnboardingProvider";
import { Button } from "@/components/ui/button";
import { GraduationCap } from "lucide-react";

export function OnboardingTrigger() {
  const { openOnboarding, hasCompletedOnboarding } = useOnboarding();

  return (
    <Button
      variant="ghost"
      size="sm"
      onClick={openOnboarding}
      title={hasCompletedOnboarding ? "Replay onboarding tour" : "Start onboarding tour"}
    >
      <GraduationCap className="h-4 w-4 mr-1" />
      Tour
    </Button>
  );
}
