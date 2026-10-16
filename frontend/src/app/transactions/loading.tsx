import { PageSkeleton, TableSkeleton } from "@/components/LoadingSkeleton";

export default function TransactionsLoading() {
  return (
    <PageSkeleton>
      <TableSkeleton rows={8} cols={5} label="Loading transactions" />
    </PageSkeleton>
  );
}
