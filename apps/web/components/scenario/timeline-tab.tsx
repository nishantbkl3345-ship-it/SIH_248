"use client";

import { ArrowDown, ArrowUp, Plus } from "lucide-react";

import { ConfirmButton } from "@/components/scenario/confirm-button";
import { ClockField, TextAreaField, TextField } from "@/components/scenario/fields";
import { KIND } from "@/components/scenario/kinds";
import { MasterTimeline, TimelineLegend } from "@/components/scenario/master-timeline";
import { TimelineList } from "@/components/scenario/timeline-list";
import type { ScenarioEditor } from "@/components/scenario/use-scenario-editor";
import { Button } from "@/components/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import {
  LIMITS,
  addItem,
  movePhase,
  newDecisionPoint,
  newPhase,
  newReport,
  newRule,
  removePhase,
  updatePhase,
} from "@/lib/scenario/edit";
import { formatClock, formatDuration } from "@/lib/scenario/time";
import { buildTimeline, phaseStarts, type TimelineKind } from "@/lib/scenario/timeline";
import type { DegradationMode, ItemRef, Phase } from "@/lib/scenario/types";

/** A sensible time for a new item: a minute after the last one, inside the phase. */
function nextSlot(phase: Phase): number {
  const used = [
    ...phase.reports.map((r) => r.atSec),
    ...phase.rules.map((r) => r.atSec),
    ...phase.decisionPoints.map((d) => d.atSec),
  ];
  if (used.length === 0) return Math.min(60, phase.durationSec);
  return Math.min(Math.max(...used) + 60, phase.durationSec);
}

const ADD_GROUPS: {
  heading: string;
  items: { label: string; kind: TimelineKind; action: "report" | "decision" | "phase" | DegradationMode }[];
}[] = [
  {
    heading: "Information",
    items: [
      { label: "New synthetic report appears", kind: "report", action: "report" },
      { label: "Conflicting reports appear", kind: "conflict", action: "CONFLICTING" },
    ],
  },
  {
    heading: "Communications",
    items: [
      { label: "Information becomes delayed", kind: "delay", action: "DELAY" },
      { label: "Information becomes unavailable", kind: "dropout", action: "DROPOUT" },
      { label: "Channel becomes degraded", kind: "partial", action: "PARTIAL" },
      { label: "Communications forced to normal", kind: "normal", action: "NORMAL" },
    ],
  },
  {
    heading: "Decisions and structure",
    items: [
      { label: "Decision point opens and closes", kind: "decision-open", action: "decision" },
      { label: "Phase transition (add a phase)", kind: "phase", action: "phase" },
    ],
  },
];

export function TimelineTab({
  editor,
  phaseId,
  onSelectPhase,
  onOpenItem,
}: {
  editor: ScenarioEditor;
  phaseId: string | undefined;
  onSelectPhase: (id: string) => void;
  onOpenItem: (ref: ItemRef) => void;
}) {
  const { content, update, issues } = editor;
  const starts = phaseStarts(content.phases);
  const index = Math.max(0, content.phases.findIndex((p) => p.id === phaseId));
  const phase: Phase | undefined = content.phases[index];

  const flagged = new Set(issues.filter((i) => i.severity === "error" && i.entityId).map((i) => i.entityId!));
  const phaseIssue = (field: string) =>
    phase && issues.find((i) => i.entityId === phase.id && i.path.endsWith(`.${field}`))?.message;

  function appendPhase() {
    const created = newPhase(content.phases.length);
    update((c) => ({ ...c, phases: [...c.phases, created] }));
    onSelectPhase(created.id);
  }

  function add(action: (typeof ADD_GROUPS)[number]["items"][number]["action"]) {
    if (action === "phase") return appendPhase();
    if (!phase) return;
    const at = nextSlot(phase);
    const channel = content.channels[0]?.id ?? null;
    if (action === "report") {
      const item = newReport(at, content.channels.find((c) => c.kind === "FEED")?.id ?? channel);
      update((c) => addItem(c, phase.id, "report", item));
      onOpenItem({ phaseId: phase.id, type: "report", id: item.id });
    } else if (action === "decision") {
      const item = newDecisionPoint(at);
      update((c) => addItem(c, phase.id, "decision", item));
      onOpenItem({ phaseId: phase.id, type: "decision", id: item.id });
    } else {
      // A conflict needs a channel to arrive on; other rules default to all channels.
      const item = newRule(action, at, action === "CONFLICTING" ? channel : null);
      update((c) => addItem(c, phase.id, "rule", item));
      onOpenItem({ phaseId: phase.id, type: "rule", id: item.id });
    }
  }

  const full =
    phase &&
    (phase.reports.length >= LIMITS.reports ||
      phase.rules.length >= LIMITS.rules ||
      phase.decisionPoints.length >= LIMITS.decisionPoints);

  const entries = phase ? buildTimeline(content).filter((e) => e.phaseId === phase.id) : [];

  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-6">
      <Card>
        <CardHeader>
          <CardTitle>Scenario timeline</CardTitle>
          <CardDescription>
            The whole exercise on one axis. Select a phase to edit it, or a marker to open that item.
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4">
          <MasterTimeline
            content={content}
            selectedPhaseId={phase?.id}
            onSelectPhase={onSelectPhase}
            onOpenItem={(ref) => {
              onSelectPhase(ref.phaseId);
              onOpenItem(ref);
            }}
          />
          <TimelineLegend />
        </CardContent>
      </Card>

      <div className="grid grid-cols-[minmax(0,1fr)] items-start gap-6 lg:grid-cols-[17rem_minmax(0,1fr)]">
        <Card>
          <CardHeader>
            <CardTitle>Phases</CardTitle>
            <CardDescription>Run in this order.</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-1.5">
            {content.phases.map((p, i) => {
              const selected = p.id === phase?.id;
              const count = p.reports.length + p.rules.length + p.decisionPoints.length;
              return (
                <button
                  key={p.id}
                  type="button"
                  aria-current={selected ? "true" : undefined}
                  onClick={() => onSelectPhase(p.id)}
                  className={cn(
                    "grid grid-cols-[1.5rem_1fr] gap-x-2 rounded-lg border p-2.5 text-left transition-colors focus-visible:outline-2 focus-visible:outline-ring",
                    selected ? "border-foreground bg-accent" : "hover:bg-accent/50",
                  )}
                >
                  <span className="row-span-2 flex size-6 items-center justify-center rounded-full bg-secondary font-mono text-xs">
                    {i + 1}
                  </span>
                  <span className="truncate text-sm font-medium">{p.title || "Untitled phase"}</span>
                  <span className="font-mono text-xs tabular-nums text-muted-foreground">
                    {formatClock(starts[i])}–{formatClock(starts[i] + p.durationSec)} · {count}{" "}
                    {count === 1 ? "item" : "items"}
                  </span>
                </button>
              );
            })}
            <Button
              type="button"
              variant="outline"
              className="mt-1.5"
              disabled={content.phases.length >= LIMITS.phases}
              onClick={appendPhase}
            >
              <Plus aria-hidden />
              Add phase
            </Button>
          </CardContent>
        </Card>

        {phase ? (
          <Card>
            <CardHeader>
              <CardTitle>
                Phase {index + 1} of {content.phases.length}
              </CardTitle>
              <CardDescription>
                {formatClock(starts[index])}–{formatClock(starts[index] + phase.durationSec)} ·{" "}
                {formatDuration(phase.durationSec)}
              </CardDescription>
              <CardAction className="flex items-center gap-1">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  disabled={index === 0}
                  onClick={() => update((c) => movePhase(c, phase.id, -1))}
                >
                  <ArrowUp aria-hidden />
                  <span className="sr-only">Move phase earlier</span>
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon-sm"
                  disabled={index === content.phases.length - 1}
                  onClick={() => update((c) => movePhase(c, phase.id, 1))}
                >
                  <ArrowDown aria-hidden />
                  <span className="sr-only">Move phase later</span>
                </Button>
                <ConfirmButton
                  iconOnly
                  label="Delete phase"
                  confirmLabel="Delete phase and its items"
                  onConfirm={() => update((c) => removePhase(c, phase.id))}
                />
              </CardAction>
            </CardHeader>
            <CardContent className="grid gap-6">
              <div className="grid gap-4 sm:grid-cols-[1fr_9rem]">
                <TextField
                  label="Phase title"
                  value={phase.title}
                  maxLength={120}
                  onChange={(title) => update((c) => updatePhase(c, phase.id, { title }))}
                  error={phaseIssue("title")}
                />
                <ClockField
                  label="Duration"
                  value={phase.durationSec}
                  onChange={(durationSec) => update((c) => updatePhase(c, phase.id, { durationSec }))}
                  hint="mm:ss"
                  error={phaseIssue("durationSec")}
                />
                <TextAreaField
                  className="sm:col-span-2"
                  label="Description"
                  value={phase.description}
                  maxLength={4000}
                  rows={2}
                  placeholder="What is happening in this part of the exercise."
                  onChange={(description) => update((c) => updatePhase(c, phase.id, { description }))}
                />
              </div>

              <div className="grid gap-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <div>
                    <h3 className="text-sm font-semibold">Events in this phase</h3>
                    <p className="text-xs text-muted-foreground">
                      Times are exercise time. Greyed rows follow from the item above them.
                    </p>
                  </div>
                  <DropdownMenu>
                    <DropdownMenuTrigger render={<Button type="button" disabled={Boolean(full)} />}>
                      <Plus aria-hidden />
                      Add event
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end" className="w-72">
                      {ADD_GROUPS.map((g, gi) => (
                        <DropdownMenuGroup key={g.heading}>
                          {gi > 0 && <DropdownMenuSeparator />}
                          <DropdownMenuLabel>{g.heading}</DropdownMenuLabel>
                          {g.items.map((it) => {
                            const Icon = KIND[it.kind].icon;
                            return (
                              <DropdownMenuItem key={it.label} onClick={() => add(it.action)}>
                                <Icon aria-hidden />
                                {it.label}
                              </DropdownMenuItem>
                            );
                          })}
                        </DropdownMenuGroup>
                      ))}
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
                <TimelineList entries={entries} onOpen={onOpenItem} flagged={flagged} />
                {entries.every((e) => !e.item) && (
                  <p className="rounded-lg border border-dashed px-4 py-6 text-center text-sm text-muted-foreground">
                    Nothing happens in this phase yet. Use <span className="font-medium">Add event</span> to
                    schedule a report, a communication problem or a decision point.
                  </p>
                )}
              </div>
            </CardContent>
          </Card>
        ) : (
          <Card>
            <CardContent className="py-10 text-center text-sm text-muted-foreground">
              This scenario has no phases. Add one to start building the timeline.
            </CardContent>
          </Card>
        )}
      </div>
    </div>
  );
}
