import { DashboardSkeleton, PageSkeleton } from "@/components/LoadingSkeleton";

export default function AnalyticsLoading() {
  return (
    <PageSkeleton>
      <DashboardSkeleton label="Loading analytics" />
    </PageSkeleton>
  );
}
