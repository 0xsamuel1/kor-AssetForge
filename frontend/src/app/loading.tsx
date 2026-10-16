import { PageSkeleton } from "@/components/LoadingSkeleton";

// Root Suspense boundary: shown while any route segment without its own
// `loading.tsx` is streaming in.
export default function Loading() {
  return <PageSkeleton />;
}
