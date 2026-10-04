"use client";

import { KIND } from "@/components/scenario/kinds";
import { cn } from "@/lib/utils";
import { formatClock, formatDuration } from "@/lib/scenario/time";
import { phaseStarts, ruleKind, ruleLabel, totalDuration } from "@/lib/scenario/timeline";
import type { ItemRef, ScenarioContent } from "@/lib/scenario/types";

interface Mark {
  key: string;
  ref: ItemRef;
  kind: keyof typeof KIND;
  startSec: number;
  /** Absent for instantaneous events. */
  endSec?: number;
  title: string;
}

function Lane({
  label,
  marks,
  total,
  onOpen,
}: {
  label: string;
  marks: Mark[];
  total: number;
  onOpen?: (ref: ItemRef) => void;
}) {
  return (
    <>
      <div className="flex items-center text-xs font-medium text-muted-foreground">{label}</div>
      <div className="relative h-7 rounded-md bg-muted/50">
        {marks.length === 0 && (
          <span className="absolute inset-0 flex items-center px-2 text-[0.7rem] text-muted-foreground/70">
            None
          </span>
        )}
        {marks.map((m) => {
          const left = `${(m.startSec / total) * 100}%`;
          const tip = `${formatClock(m.startSec)}${m.endSec !== undefined ? `–${formatClock(m.endSec)}` : ""}  ${m.title}`;
          const common = {
            type: "button" as const,
            title: tip,
            "aria-label": tip,
            disabled: !onOpen,
            onClick: () => onOpen?.(m.ref),
          };
          return m.endSec === undefined ? (
            <button
              key={m.key}
              {...common}
              style={{ left }}
              className={cn(
                "absolute top-1/2 size-3 -translate-x-1/2 -translate-y-1/2 rotate-45 rounded-[2px] ring-2 ring-background transition-transform enabled:hover:scale-125 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
                KIND[m.kind].solid,
              )}
            />
          ) : (
            <button
              key={m.key}
              {...common}
              style={{ left, width: `max(${((m.endSec - m.startSec) / total) * 100}%, 6px)` }}
              className={cn(
                "absolute inset-y-1 rounded-sm ring-1 ring-inset transition-[filter] enabled:hover:brightness-90 focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-ring",
                KIND[m.kind].band,
              )}
            />
          );
        })}
      </div>
    </>
  );
}

/**
 * The whole scenario on one time axis: phases across the top, then one lane
 * each for reports, communications and decisions.
 */
export function MasterTimeline({
  content,
  selectedPhaseId,
  onSelectPhase,
  onOpenItem,
}: {
  content: ScenarioContent;
  selectedPhaseId?: string;
  onSelectPhase?: (phaseId: string) => void;
  onOpenItem?: (ref: ItemRef) => void;
}) {
  const total = totalDuration(content.phases);
  if (total <= 0) {
    return (
      <p className="rounded-md border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
        Give at least one phase a duration to see the timeline.
      </p>
    );
  }

  const starts = phaseStarts(content.phases);
  const reports: Mark[] = [];
  const comms: Mark[] = [];
  const decisions: Mark[] = [];

  content.phases.forEach((p, i) => {
    const start = starts[i];
    const end = start + p.durationSec;
    const clip = (t: number) => Math.min(Math.max(t, start), end);
    for (const r of p.reports) {
      reports.push({
        key: r.id,
        ref: { phaseId: p.id, type: "report", id: r.id },
        kind: "report",
        startSec: clip(start + r.atSec),
        title: r.title.trim() || "Untitled report",
      });
    }
    for (const r of p.rules) {
      const at = clip(start + r.atSec);
      comms.push({
        key: r.id,
        ref: { phaseId: p.id, type: "rule", id: r.id },
        kind: ruleKind(r),
        startSec: at,
        endSec: r.mode === "CONFLICTING" ? undefined : clip(at + r.durationSec),
        title: ruleLabel(r),
      });
    }
    for (const d of p.decisionPoints) {
      const at = clip(start + d.atSec);
      decisions.push({
        key: d.id,
        ref: { phaseId: p.id, type: "decision", id: d.id },
        kind: "decision-open",
        startSec: at,
        endSec: Math.min(at + d.deadlineSec, total),
        title: d.question.trim() || "Untitled decision",
      });
    }
  });

  const ticks = [...starts, total];

  return (
    <div className="overflow-x-auto">
      <div className="grid min-w-[640px] grid-cols-[5.5rem_1fr] gap-x-3 gap-y-1.5">
        <div className="flex items-center text-xs font-medium text-muted-foreground">Phases</div>
        <div className="flex h-9 gap-px overflow-hidden rounded-md">
          {content.phases.map((p, i) => {
            const selected = p.id === selectedPhaseId;
            return (
              <button
                key={p.id}
                type="button"
                disabled={!onSelectPhase}
                onClick={() => onSelectPhase?.(p.id)}
                aria-pressed={onSelectPhase ? selected : undefined}
                title={`${p.title || `Phase ${i + 1}`} · ${formatDuration(p.durationSec)}`}
                style={{ flexGrow: p.durationSec, flexBasis: 0 }}
                className={cn(
                  "flex min-w-0 items-center gap-1.5 px-2 text-left text-xs font-medium transition-colors focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring",
                  selected
                    ? "bg-primary text-primary-foreground"
                    : "bg-secondary text-secondary-foreground enabled:hover:bg-accent",
                )}
              >
                <span className="font-mono tabular-nums opacity-70">{i + 1}</span>
                <span className="truncate">{p.title || "Untitled"}</span>
              </button>
            );
          })}
        </div>

        <Lane label="Reports" marks={reports} total={total} onOpen={onOpenItem} />
        <Lane label="Comms" marks={comms} total={total} onOpen={onOpenItem} />
        <Lane label="Decisions" marks={decisions} total={total} onOpen={onOpenItem} />

        <div />
        <div className="relative h-5">
          {ticks.map((t, i) => (
            <span
              key={`${t}-${i}`}
              style={{ left: `${(t / total) * 100}%` }}
              className={cn(
                "absolute top-0 font-mono text-[0.7rem] tabular-nums text-muted-foreground",
                i === 0 ? "" : i === ticks.length - 1 ? "-translate-x-full" : "-translate-x-1/2",
              )}
            >
              {formatClock(t)}
            </span>
          ))}
        </div>
      </div>
    </div>
  );
}

const LEGEND = ["report", "delay", "dropout", "partial", "conflict", "normal", "decision-open"] as const;

export function TimelineLegend() {
  return (
    <ul className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
      {LEGEND.map((k) => {
        const Icon = KIND[k].icon;
        return (
          <li key={k} className="flex items-center gap-1.5">
            <span className={cn("flex size-5 items-center justify-center rounded-full", KIND[k].chip)}>
              <Icon className="size-3" aria-hidden />
            </span>
            {KIND[k].name}
          </li>
        );
      })}
    </ul>
  );
}
