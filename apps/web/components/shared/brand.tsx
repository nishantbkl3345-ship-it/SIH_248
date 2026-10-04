import { RadioTower } from "lucide-react";

import { cn } from "@/lib/utils";

export function Brand({ className }: { className?: string }) {
  return (
    <span className={cn("flex items-center gap-2 font-semibold tracking-tight", className)}>
      <span className="flex size-7 items-center justify-center rounded-md bg-primary text-primary-foreground">
        <RadioTower className="size-4" aria-hidden />
      </span>
      {process.env.NEXT_PUBLIC_APP_NAME || "FOGLINE"}
    </span>
  );
}
