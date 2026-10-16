import { DetailSkeleton, PageSkeleton } from "@/components/LoadingSkeleton";

export default function TradeLoading() {
  return (
    <PageSkeleton>
      <DetailSkeleton label="Loading trading pair" />
    </PageSkeleton>
  );
}
