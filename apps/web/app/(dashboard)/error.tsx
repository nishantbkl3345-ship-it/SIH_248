"use client";

import { useEffect } from "react";

import { ErrorState } from "@/components/shared/error-state";

// Rendered inside the dashboard shell, so navigation stays usable.
export default function DashboardError({
  error,
  retry,
}: {
  error: Error & { digest?: string };
  retry: () => void;
}) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return <ErrorState digest={error.digest} onRetry={retry} />;
}
