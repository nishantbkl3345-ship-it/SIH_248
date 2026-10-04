"use client";

import { ChevronRight, TriangleAlert } from "lucide-react";

import { KIND } from "@/components/scenario/kinds";
import { cn } from "@/lib/utils";
import { formatClock } from "@/lib/scenario/time";
import type { TimelineEntry } from "@/lib/scenario/timeline";
import type { ItemRef } from "@/lib/scenario/types";

/**
 * Chronological list: clock on the left, a kind marker on a connecting rail,
 * then what happens. Rows backed by an authored item open it when clicked.
 */
export function TimelineList({
  entries,
  onOpen,
  flagged,
  renderExtra,
}: {
  entries: TimelineEntry[];
  onOpen?: (ref: ItemRef) => void;
  /** Item ids that have outstanding issues. */
  flagged?: Set<string>;
  renderExtra?: (entry: TimelineEntry) => React.ReactNode;
}) {
  return (
    <ol className="relative">
      {entries.map((e, i) => {
        const kind = KIND[e.kind];
        const Icon = kind.icon;
        const clickable = Boolean(onOpen && e.item);
        const extra = renderExtra?.(e);
        const hasIssue = e.item && !e.derived && flagged?.has(e.item.id);

        const body = (
          <>
            <span className="flex min-w-0 flex-1 flex-col">
              <span
                className={cn(
                  "text-sm",
                  e.derived ? "text-muted-foreground" : "font-medium",
                )}
              >
                {e.label}
              </span>
              {e.detail && <span className="truncate text-xs text-muted-foreground">{e.detail}</span>}
            </span>
            {hasIssue && (
              <span className="flex items-center gap-1 text-xs font-medium text-destructive">
                <TriangleAlert className="size-3.5" aria-hidden />
                Needs attention
              </span>
            )}
            {clickable && <ChevronRight className="size-4 shrink-0 text-muted-foreground" aria-hidden />}
          </>
        );

        return (
          <li key={e.key} className="grid grid-cols-[3.75rem_1.75rem_1fr] gap-x-2">
            <time className="pt-2 text-right font-mono text-xs tabular-nums text-muted-foreground">
              {formatClock(e.atSec)}
            </time>
            <div className="relative flex justify-center">
              {i < entries.length - 1 && (
                <span className="absolute top-4 -bottom-0 w-px bg-border" aria-hidden />
              )}
              <span
                className={cn(
                  "relative z-10 mt-1 flex size-6 items-center justify-center rounded-full",
                  kind.chip,
                )}
                title={kind.name}
              >
                <Icon className="size-3.5" aria-hidden />
              </span>
            </div>
            <div className="min-w-0 pb-3">
              {clickable ? (
                <button
                  type="button"
                  onClick={() => onOpen!({ phaseId: e.phaseId, ...e.item! })}
                  className="flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring"
                >
                  {body}
                </button>
              ) : (
                <div className="flex items-center gap-2 px-2 py-1.5">{body}</div>
              )}
              {extra && <div className="px-2 pt-1">{extra}</div>}
            </div>
          </li>
        );
      })}
    </ol>
  );
}
