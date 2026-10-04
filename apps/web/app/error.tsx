"use client";

import { useEffect } from "react";

import { ErrorState } from "@/components/shared/error-state";

export default function RootError({
  error,
  retry,
}: {
  error: Error & { digest?: string };
  retry: () => void;
}) {
  useEffect(() => {
    console.error(error);
  }, [error]);

  return (
    <div className="flex flex-1 items-center justify-center">
      <ErrorState digest={error.digest} onRetry={retry} />
    </div>
  );
}
