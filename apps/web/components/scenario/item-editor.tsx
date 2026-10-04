"use client";

import { Plus, X } from "lucide-react";

import { ConfirmButton } from "@/components/scenario/confirm-button";
import {
  ClockField,
  NumberField,
  SelectField,
  TextAreaField,
  TextField,
} from "@/components/scenario/fields";
import { KIND, MODES, PRIORITY_LABEL, RELIABILITY_LABEL } from "@/components/scenario/kinds";
import type { ScenarioEditor } from "@/components/scenario/use-scenario-editor";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { cn } from "@/lib/utils";
import { LIMITS, changeRuleMode, findItem, newOption, removeItem, updateItem } from "@/lib/scenario/edit";
import { formatClock } from "@/lib/scenario/time";
import { phaseStarts } from "@/lib/scenario/timeline";
import type {
  Conflict,
  DecisionPoint,
  DecisionScope,
  ItemRef,
  Priority,
  Reliability,
  Report,
  Rule,
  ScenarioContent,
} from "@/lib/scenario/types";

const TITLES = {
  report: ["Information report", "A synthetic report delivered to trainees at a set time."],
  rule: ["Communication rule", "How information is degraded, and for whom, during a window of this phase."],
  decision: ["Decision point", "A question trainees must answer before a deadline."],
} as const;

const ALL = "__all__";

function teamOptions(count: number, allLabel: string) {
  return [
    { value: ALL, label: allLabel },
    ...Array.from({ length: count }, (_, i) => ({ value: String(i + 1), label: `Team ${i + 1}` })),
  ];
}

function channelOptions(content: ScenarioContent, emptyLabel: string) {
  return [
    { value: ALL, label: emptyLabel },
    ...content.channels.map((c) => ({ value: c.id, label: c.name.trim() || "Unnamed channel" })),
  ];
}

const fromSelect = (v: string) => (v === ALL ? null : v);
const teamFromSelect = (v: string) => (v === ALL ? null : Number(v));

interface FormProps<T> {
  item: T;
  set: (change: Partial<T> | ((item: T) => T)) => void;
  content: ScenarioContent;
  err: (field: string) => string | undefined;
  /** Hint under a time field, showing the absolute exercise time. */
  when: (offsetSec: number) => string;
  phaseDuration: number;
}

function ReportForm({ item, set, content, err, when, phaseDuration }: FormProps<Report>) {
  return (
    <>
      <TextField
        label="Title"
        value={item.title}
        onChange={(title) => set({ title })}
        maxLength={120}
        placeholder="Short label shown on the timeline"
        error={err("title")}
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <TextField
          label="Source"
          value={item.source}
          onChange={(source) => set({ source })}
          maxLength={120}
          placeholder="A fictional source, e.g. Relay Station K-7"
          error={err("source")}
        />
        <ClockField
          label="Arrives at"
          value={item.atSec}
          max={phaseDuration || undefined}
          onChange={(atSec) => set({ atSec })}
          hint={when(item.atSec)}
          error={err("atSec")}
        />
      </div>
      <TextAreaField
        label="Message"
        value={item.content}
        onChange={(content) => set({ content })}
        maxLength={4000}
        rows={5}
        placeholder="What the report says. Fictional content only."
        error={err("content")}
      />
      <div className="grid gap-4 sm:grid-cols-2">
        <SelectField<Priority>
          label="Priority"
          value={item.priority}
          onChange={(priority) => set({ priority })}
          options={(Object.keys(PRIORITY_LABEL) as Priority[]).map((v) => ({ value: v, label: PRIORITY_LABEL[v] }))}
        />
        <SelectField<Reliability>
          label="Source reliability"
          value={item.reliability}
          onChange={(reliability) => set({ reliability })}
          options={(Object.keys(RELIABILITY_LABEL) as Reliability[]).map((v) => ({
            value: v,
            label: RELIABILITY_LABEL[v],
          }))}
          hint="The rating trainees see next to the source."
        />
        <SelectField
          label="Target team"
          value={item.targetTeam === null ? ALL : String(item.targetTeam)}
          onChange={(v) => set({ targetTeam: teamFromSelect(v) })}
          options={teamOptions(content.teamCount, "All teams")}
          error={err("targetTeam")}
        />
        <SelectField
          label="Delivery channel"
          value={item.channelId ?? ALL}
          onChange={(v) => set({ channelId: fromSelect(v) })}
          options={channelOptions(content, "Choose a channel…")}
          error={err("channelId")}
        />
      </div>
    </>
  );
}

function RuleForm({ item, set, content, err, when, phaseDuration }: FormProps<Rule>) {
  const conflict = item.conflict;
  const setConflict = (patch: Partial<Conflict>) =>
    set((r) => ({ ...r, conflict: { ...(r.conflict ?? { topic: "", sourceA: "", contentA: "", sourceB: "", contentB: "" }), ...patch } }));

  return (
    <>
      <fieldset className="grid gap-1.5">
        <legend className="mb-1.5 text-sm font-medium">Mode</legend>
        <div className="grid gap-2 sm:grid-cols-2">
          {MODES.map((m) => {
            const Icon = KIND[m.kind].icon;
            const active = item.mode === m.mode;
            return (
              <button
                key={m.mode}
                type="button"
                aria-pressed={active}
                onClick={() => set((r) => changeRuleMode(r, m.mode))}
                className={cn(
                  "flex items-start gap-2.5 rounded-lg border p-2.5 text-left transition-colors focus-visible:outline-2 focus-visible:outline-ring",
                  active ? "border-foreground bg-accent" : "hover:bg-accent/50",
                )}
              >
                <span className={cn("mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full", KIND[m.kind].chip)}>
                  <Icon className="size-3.5" aria-hidden />
                </span>
                <span>
                  <span className="block text-sm font-medium">{m.title}</span>
                  <span className="block text-xs text-muted-foreground">{m.description}</span>
                </span>
              </button>
            );
          })}
        </div>
      </fieldset>

      <TextField
        label="Label"
        value={item.name}
        onChange={(name) => set({ name })}
        maxLength={120}
        placeholder="Optional note for yourself, e.g. Relay outage"
      />

      <div className="grid gap-4 sm:grid-cols-2">
        <ClockField
          label={item.mode === "CONFLICTING" ? "Reports arrive at" : "Start time"}
          value={item.atSec}
          max={phaseDuration || undefined}
          onChange={(atSec) => set({ atSec })}
          hint={when(item.atSec)}
          error={err("atSec")}
        />
        {item.mode !== "CONFLICTING" && (
          <ClockField
            label="Duration"
            value={item.durationSec}
            onChange={(durationSec) => set({ durationSec })}
            hint={`Until ${when(item.atSec + item.durationSec).replace("Exercise time ", "")} exercise time`}
            error={err("durationSec")}
          />
        )}
        <SelectField
          label={item.mode === "CONFLICTING" ? "Delivery channel" : "Affected channel"}
          value={item.channelId ?? ALL}
          onChange={(v) => set({ channelId: fromSelect(v) })}
          options={channelOptions(content, item.mode === "CONFLICTING" ? "Choose a channel…" : "All channels")}
          error={err("channelId")}
        />
        <SelectField
          label="Affected team"
          value={item.targetTeam === null ? ALL : String(item.targetTeam)}
          onChange={(v) => set({ targetTeam: teamFromSelect(v) })}
          options={teamOptions(content.teamCount, "All teams")}
          error={err("targetTeam")}
        />
      </div>

      {item.mode === "DELAY" && (
        <ClockField
          label="Delay duration"
          value={item.delaySec}
          max={3600}
          onChange={(delaySec) => set({ delaySec })}
          hint="How late each item arrives while this rule is in force."
          error={err("delaySec")}
        />
      )}
      {item.mode === "PARTIAL" && (
        <NumberField
          label="Content delivered (%)"
          value={item.keepPercent}
          min={0}
          max={100}
          onChange={(keepPercent) => set({ keepPercent })}
          hint="The share of each item that gets through; the rest is lost."
          error={err("keepPercent")}
        />
      )}
      {item.mode === "DROPOUT" && (
        <p className="rounded-lg border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
          Everything sent on the affected channel during this window is lost. Trainees are not told.
        </p>
      )}
      {item.mode === "NORMAL" && (
        <p className="rounded-lg border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
          Overrides any other rule on the affected channel for this window, so communications are clear.
        </p>
      )}
      {item.mode === "CONFLICTING" && conflict && (
        <>
          <TextField
            label="Subject"
            value={conflict.topic}
            onChange={(topic) => setConflict({ topic })}
            maxLength={120}
            placeholder="What the two reports disagree about"
          />
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid content-start gap-3 rounded-lg border p-3">
              <TextField
                label="Source A"
                value={conflict.sourceA}
                onChange={(sourceA) => setConflict({ sourceA })}
                maxLength={120}
                error={err("conflict.sourceA")}
              />
              <TextAreaField
                label="Source A reports"
                value={conflict.contentA}
                onChange={(contentA) => setConflict({ contentA })}
                maxLength={2000}
                error={err("conflict.contentA")}
              />
            </div>
            <div className="grid content-start gap-3 rounded-lg border p-3">
              <TextField
                label="Source B"
                value={conflict.sourceB}
                onChange={(sourceB) => setConflict({ sourceB })}
                maxLength={120}
                error={err("conflict.sourceB")}
              />
              <TextAreaField
                label="Source B reports"
                value={conflict.contentB}
                onChange={(contentB) => setConflict({ contentB })}
                maxLength={2000}
                error={err("conflict.contentB")}
              />
            </div>
          </div>
        </>
      )}
    </>
  );
}

function DecisionForm({ item, set, err, when, phaseDuration }: FormProps<DecisionPoint>) {
  return (
    <>
      <TextAreaField
        label="Question"
        value={item.question}
        onChange={(question) => set({ question })}
        maxLength={1000}
        rows={2}
        placeholder="What must trainees decide?"
        error={err("question")}
      />
      <div className="grid gap-4 sm:grid-cols-3">
        <ClockField
          label="Opens at"
          value={item.atSec}
          max={phaseDuration || undefined}
          onChange={(atSec) => set({ atSec })}
          hint={when(item.atSec)}
          error={err("atSec")}
        />
        <ClockField
          label="Time to decide"
          value={item.deadlineSec}
          onChange={(deadlineSec) => set({ deadlineSec })}
          hint={`Deadline ${when(item.atSec + item.deadlineSec).replace("Exercise time ", "")}`}
          error={err("deadlineSec")}
        />
        <SelectField<DecisionScope>
          label="Decided by"
          value={item.scope}
          onChange={(scope) => set({ scope })}
          options={[
            { value: "TEAM", label: "Each team" },
            { value: "INDIVIDUAL", label: "Each trainee" },
          ]}
        />
      </div>

      <fieldset className="grid gap-2">
        <legend className="mb-1.5 text-sm font-medium">Available options</legend>
        {item.options.map((o, i) => (
          <div key={o.id} className="grid grid-cols-[1.25rem_1fr_auto] items-start gap-2">
            <span className="pt-1.5 text-right font-mono text-xs text-muted-foreground">
              {String.fromCharCode(65 + i)}
            </span>
            <div className="grid gap-1.5">
              <Input
                aria-label={`Option ${String.fromCharCode(65 + i)}`}
                placeholder="Option"
                value={o.label}
                maxLength={200}
                aria-invalid={err(`options[${i}].label`) ? true : undefined}
                onChange={(e) =>
                  set((d) => ({
                    ...d,
                    options: d.options.map((x) => (x.id === o.id ? { ...x, label: e.target.value } : x)),
                  }))
                }
              />
              {err(`options[${i}].label`) && (
                <p className="text-xs text-destructive">{err(`options[${i}].label`)}</p>
              )}
            </div>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              disabled={item.options.length <= 2}
              onClick={() => set((d) => ({ ...d, options: d.options.filter((x) => x.id !== o.id) }))}
            >
              <X aria-hidden />
              <span className="sr-only">Remove option {String.fromCharCode(65 + i)}</span>
            </Button>
          </div>
        ))}
        {err("options") && <p className="text-xs text-destructive">{err("options")}</p>}
        <div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            disabled={item.options.length >= LIMITS.options}
            onClick={() => set((d) => ({ ...d, options: [...d.options, newOption()] }))}
          >
            <Plus aria-hidden />
            Add option
          </Button>
        </div>
      </fieldset>

      <label className="flex items-start gap-2.5 rounded-lg border p-3 text-sm">
        <input
          type="checkbox"
          className="mt-0.5 size-4 accent-foreground"
          checked={item.rationaleRequired}
          onChange={(e) => set({ rationaleRequired: e.target.checked })}
        />
        <span>
          <span className="block font-medium">Rationale required</span>
          <span className="block text-xs text-muted-foreground">
            Trainees must explain their reasoning before they can submit.
          </span>
        </span>
      </label>

      <TextAreaField
        label="Evaluation criteria"
        value={item.evaluationCriteria}
        onChange={(evaluationCriteria) => set({ evaluationCriteria })}
        maxLength={4000}
        rows={4}
        placeholder="What a sound decision looks like here. Shown to instructors in the review, never to trainees."
        error={err("evaluationCriteria")}
      />
    </>
  );
}

/** Side panel for editing the selected timeline item. Changes apply at once. */
export function ItemEditor({
  editor,
  selected,
  onClose,
}: {
  editor: ScenarioEditor;
  selected: ItemRef | null;
  onClose: () => void;
}) {
  const { content, update, issues } = editor;
  const phaseIndex = selected ? content.phases.findIndex((p) => p.id === selected.phaseId) : -1;
  const phase = content.phases[phaseIndex];
  const item = selected ? findItem(content, selected.phaseId, selected.type, selected.id) : undefined;
  const open = Boolean(selected && phase && item);

  const phaseStart = phaseIndex >= 0 ? phaseStarts(content.phases)[phaseIndex] : 0;
  const mine = issues.filter((i) => selected && i.entityId === selected.id);
  const err = (field: string) => mine.find((i) => i.path.endsWith(`.${field}`))?.message;
  const when = (offsetSec: number) => `Exercise time ${formatClock(phaseStart + offsetSec)}`;

  const common = { content, err, when, phaseDuration: phase?.durationSec ?? 0 };
  const setter =
    <T,>() =>
    (change: Partial<T> | ((item: T) => T)) =>
      selected &&
      update((c) => updateItem(c, selected.phaseId, selected.type, selected.id, change as never));

  return (
    <Sheet open={open} onOpenChange={(next) => !next && onClose()}>
      <SheetContent className="w-full gap-0 overflow-y-auto data-[side=right]:sm:max-w-xl">
        {open && selected && item && phase && (
          <>
            <SheetHeader>
              <SheetTitle>{TITLES[selected.type][0]}</SheetTitle>
              <SheetDescription>
                {TITLES[selected.type][1]} In phase {phaseIndex + 1}: {phase.title || "Untitled"}.
              </SheetDescription>
            </SheetHeader>
            <div className="grid gap-4 px-4 pb-4">
              {selected.type === "report" && (
                <ReportForm item={item as Report} set={setter<Report>()} {...common} />
              )}
              {selected.type === "rule" && <RuleForm item={item as Rule} set={setter<Rule>()} {...common} />}
              {selected.type === "decision" && (
                <DecisionForm item={item as DecisionPoint} set={setter<DecisionPoint>()} {...common} />
              )}
            </div>
            <SheetFooter className="sticky bottom-0 flex-row items-center justify-between border-t bg-popover">
              <ConfirmButton
                label="Delete"
                onConfirm={() => {
                  update((c) => removeItem(c, selected.phaseId, selected.type, selected.id));
                  onClose();
                }}
              />
              <Button type="button" onClick={onClose}>
                Done
              </Button>
            </SheetFooter>
          </>
        )}
      </SheetContent>
    </Sheet>
  );
}
