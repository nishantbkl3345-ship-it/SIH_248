import Link from "next/link";

import { buttonVariants } from "@/components/ui/button";

export default function NotFound() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-3 px-4 py-24 text-center">
      <p className="font-mono text-sm text-muted-foreground">404</p>
      <h1 className="text-xl font-semibold">Page not found</h1>
      <p className="text-sm text-muted-foreground">
        This page does not exist or has not been built yet.
      </p>
      <Link href="/" className={buttonVariants({ variant: "outline" })}>
        Go to dashboard
      </Link>
    </div>
  );
}
