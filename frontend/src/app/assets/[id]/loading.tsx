import { DetailSkeleton, PageSkeleton } from "@/components/LoadingSkeleton";

export default function AssetLoading() {
  return (
    <PageSkeleton>
      <DetailSkeleton label="Loading asset" />
    </PageSkeleton>
  );
}
