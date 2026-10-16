import { DashboardSkeleton, PageSkeleton } from "@/components/LoadingSkeleton";

export default function DashboardLoading() {
  return (
    <PageSkeleton>
      <DashboardSkeleton label="Loading portfolio" />
    </PageSkeleton>
  );
}
