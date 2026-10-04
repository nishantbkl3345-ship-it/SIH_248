import { formatDuration } from "@/lib/scenario/time";
import type { Channel, ItemType, Phase, Rule, ScenarioContent } from "@/lib/scenario/types";

export type TimelineKind =
  | "start"
  | "end"
  | "phase"
  | "report"
  | "delay"
  | "dropout"
  | "partial"
  | "conflict"
  | "normal"
  | "restore"
  | "decision-open"
  | "decision-close";

export interface TimelineEntry {
  key: string;
  /** Seconds from the start of the exercise. */
  atSec: number;
  kind: TimelineKind;
  label: string;
  detail?: string;
  phaseId: string;
  /** The authored item behind this entry, if any; derived entries share it. */
  item?: { type: ItemType; id: string };
  /** Team the entry is restricted to; null/undefined means everyone. */
  targetTeam?: number | null;
  /** True for entries implied by another one (a window ending, a deadline). */
  derived?: boolean;
}

/** Start time of each phase, in exercise seconds. */
export function phaseStarts(phases: Phase[]): number[] {
  const starts: number[] = [];
  let t = 0;
  for (const p of phases) {
    starts.push(t);
    t += p.durationSec;
  }
  return starts;
}

export function totalDuration(phases: Phase[]): number {
  return phases.reduce((sum, p) => sum + p.durationSec, 0);
}

export function channelName(channels: Channel[], id: string | null, fallback = "all channels"): string {
  if (id === null) return fallback;
  return channels.find((c) => c.id === id)?.name.trim() || "unnamed channel";
}

export function teamLabel(team: number | null): string {
  return team === null ? "all teams" : `Team ${team}`;
}

const RULE_KIND: Record<Rule["mode"], TimelineKind> = {
  NORMAL: "normal",
  DELAY: "delay",
  DROPOUT: "dropout",
  PARTIAL: "partial",
  CONFLICTING: "conflict",
};

export function ruleKind(rule: Rule): TimelineKind {
  return RULE_KIND[rule.mode];
}

/** One-line description of what a rule does. */
export function ruleLabel(rule: Rule): string {
  switch (rule.mode) {
    case "DELAY":
      return `Information delayed by ${formatDuration(rule.delaySec)}`;
    case "DROPOUT":
      return "Information unavailable";
    case "PARTIAL":
      return `Channel degraded: ${rule.keepPercent}% of content gets through`;
    case "CONFLICTING":
      return `Conflicting reports${rule.conflict?.topic.trim() ? `: ${rule.conflict.topic.trim()}` : ""}`;
    case "NORMAL":
      return "Communications forced to normal";
  }
}

function ruleEndLabel(rule: Rule): string {
  switch (rule.mode) {
    case "DELAY":
      return "Delay ends";
    case "DROPOUT":
      return "Information available again";
    case "PARTIAL":
      return "Channel restored";
    default:
      return "Forced-normal window ends";
  }
}

// Ties at the same instant read in this order.
const ORDER: TimelineKind[] = [
  "start",
  "phase",
  "restore",
  "decision-close",
  "normal",
  "delay",
  "dropout",
  "partial",
  "report",
  "conflict",
  "decision-open",
  "end",
];

/**
 * The scenario as a single chronological list. Everything on it is derived
 * from phases, reports, rules and decision points; nothing is stored twice.
 * Windows that would run past their phase are cut off at the phase boundary.
 */
export function buildTimeline(scenario: ScenarioContent): TimelineEntry[] {
  const entries: TimelineEntry[] = [];
  const starts = phaseStarts(scenario.phases);
  const scope = (channelId: string | null, team: number | null) =>
    `${channelName(scenario.channels, channelId)} · ${teamLabel(team)}`;

  scenario.phases.forEach((phase, i) => {
    const start = starts[i];
    const end = start + phase.durationSec;
    const title = phase.title.trim() || `Phase ${i + 1}`;

    entries.push(
      i === 0
        ? { key: "start", atSec: 0, kind: "start", label: "Exercise begins", detail: title, phaseId: phase.id }
        : {
            key: `phase:${phase.id}`,
            atSec: start,
            kind: "phase",
            label: `Phase transition: ${title}`,
            phaseId: phase.id,
          },
    );

    for (const r of phase.reports) {
      entries.push({
        key: `report:${r.id}`,
        atSec: start + r.atSec,
        kind: "report",
        label: `${r.title.trim() || "Untitled report"} arrives`,
        detail: `${r.source.trim() || "No source"} · ${scope(r.channelId, r.targetTeam).replace("all channels", "no channel")}`,
        phaseId: phase.id,
        item: { type: "report", id: r.id },
        targetTeam: r.targetTeam,
      });
    }

    for (const rule of phase.rules) {
      const item = { type: "rule" as const, id: rule.id };
      const at = start + rule.atSec;
      entries.push({
        key: `rule:${rule.id}`,
        atSec: at,
        kind: ruleKind(rule),
        label: ruleLabel(rule),
        detail: [rule.name.trim(), scope(rule.channelId, rule.targetTeam)].filter(Boolean).join(" · "),
        phaseId: phase.id,
        item,
        targetTeam: rule.targetTeam,
      });
      if (rule.mode !== "CONFLICTING" && rule.durationSec > 0) {
        entries.push({
          key: `rule-end:${rule.id}`,
          atSec: Math.min(at + rule.durationSec, end),
          kind: "restore",
          label: ruleEndLabel(rule),
          detail: scope(rule.channelId, rule.targetTeam),
          phaseId: phase.id,
          item,
          targetTeam: rule.targetTeam,
          derived: true,
        });
      }
    }

    for (const d of phase.decisionPoints) {
      const item = { type: "decision" as const, id: d.id };
      const question = d.question.trim() || "Untitled decision";
      const at = start + d.atSec;
      entries.push({
        key: `decision:${d.id}`,
        atSec: at,
        kind: "decision-open",
        label: "Decision point opens",
        detail: question,
        phaseId: phase.id,
        item,
      });
      if (d.deadlineSec > 0) {
        entries.push({
          key: `decision-close:${d.id}`,
          atSec: at + d.deadlineSec,
          kind: "decision-close",
          label: "Decision point closes",
          detail: question,
          phaseId: phase.id,
          item,
          derived: true,
        });
      }
    }
  });

  if (scenario.phases.length > 0) {
    entries.push({
      key: "end",
      atSec: totalDuration(scenario.phases),
      kind: "end",
      label: "Exercise ends",
      phaseId: scenario.phases[scenario.phases.length - 1].id,
    });
  }

  // Array.prototype.sort is stable, so authored order breaks remaining ties.
  return entries.sort((a, b) => a.atSec - b.atSec || ORDER.indexOf(a.kind) - ORDER.indexOf(b.kind));
}

/** Entries a given team would be affected by; null keeps everything. */
export function visibleToTeam(entries: TimelineEntry[], team: number | null): TimelineEntry[] {
  if (team === null) return entries;
  return entries.filter((e) => e.targetTeam == null || e.targetTeam === team);
}
