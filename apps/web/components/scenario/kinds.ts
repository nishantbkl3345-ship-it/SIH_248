import {
  FileText,
  Flag,
  Hourglass,
  ListChecks,
  Milestone,
  Play,
  RotateCcw,
  SignalMedium,
  Split,
  TimerOff,
  Wifi,
  WifiOff,
  type LucideIcon,
} from "lucide-react";

import type { TimelineKind } from "@/lib/scenario/timeline";
import type { DegradationMode } from "@/lib/scenario/types";

/**
 * How each kind of timeline entry looks. Kinds are always shown with an icon
 * and a text label as well, so colour is never the only signal.
 */
export const KIND: Record<
  TimelineKind,
  { name: string; icon: LucideIcon; chip: string; solid: string; band: string }
> = {
  start: { name: "Start", icon: Play, chip: "bg-foreground text-background", solid: "bg-foreground", band: "" },
  end: { name: "End", icon: Flag, chip: "bg-foreground text-background", solid: "bg-foreground", band: "" },
  phase: {
    name: "Phase",
    icon: Milestone,
    chip: "bg-secondary text-foreground ring-1 ring-border",
    solid: "bg-foreground/60",
    band: "",
  },
  report: {
    name: "Report",
    icon: FileText,
    chip: "bg-sky-100 text-sky-800",
    solid: "bg-sky-600",
    band: "bg-sky-500/30 ring-sky-600/50",
  },
  delay: {
    name: "Delay",
    icon: Hourglass,
    chip: "bg-amber-100 text-amber-800",
    solid: "bg-amber-500",
    band: "bg-amber-400/40 ring-amber-500/60",
  },
  dropout: {
    name: "Dropout",
    icon: WifiOff,
    chip: "bg-red-100 text-red-800",
    solid: "bg-red-600",
    band: "bg-red-500/40 ring-red-600/60",
  },
  partial: {
    name: "Partial",
    icon: SignalMedium,
    chip: "bg-orange-100 text-orange-800",
    solid: "bg-orange-500",
    band: "bg-orange-400/40 ring-orange-500/60",
  },
  conflict: {
    name: "Conflict",
    icon: Split,
    chip: "bg-violet-100 text-violet-800",
    solid: "bg-violet-600",
    band: "bg-violet-500/40 ring-violet-600/60",
  },
  normal: {
    name: "Normal",
    icon: Wifi,
    chip: "bg-emerald-100 text-emerald-800",
    solid: "bg-emerald-600",
    band: "bg-emerald-500/30 ring-emerald-600/50",
  },
  restore: {
    name: "Restored",
    icon: RotateCcw,
    chip: "bg-muted text-muted-foreground",
    solid: "bg-muted-foreground/50",
    band: "",
  },
  "decision-open": {
    name: "Decision",
    icon: ListChecks,
    chip: "bg-indigo-100 text-indigo-800",
    solid: "bg-indigo-600",
    band: "bg-indigo-500/30 ring-indigo-600/50",
  },
  "decision-close": {
    name: "Deadline",
    icon: TimerOff,
    chip: "bg-muted text-muted-foreground",
    solid: "bg-muted-foreground/50",
    band: "",
  },
};

export const MODES: { mode: DegradationMode; kind: TimelineKind; title: string; description: string }[] = [
  { mode: "DELAY", kind: "delay", title: "Delay", description: "Information arrives late by a fixed time." },
  { mode: "DROPOUT", kind: "dropout", title: "Dropout", description: "Nothing gets through for a period." },
  { mode: "PARTIAL", kind: "partial", title: "Partial", description: "Only part of each item gets through." },
  {
    mode: "CONFLICTING",
    kind: "conflict",
    title: "Conflicting",
    description: "Two sources report contradictory things.",
  },
  { mode: "NORMAL", kind: "normal", title: "Normal", description: "Force clear communications for a period." },
];

export const DIFFICULTY_LABEL = { BASIC: "Basic", INTERMEDIATE: "Intermediate", ADVANCED: "Advanced" } as const;

export const CHANNEL_KIND_LABEL = {
  TEAM: "Team (within one team)",
  CROSS_TEAM: "Cross-team",
  BROADCAST: "Broadcast (instructor to all)",
  FEED: "Feed (incoming reports)",
} as const;

export const PRIORITY_LABEL = { LOW: "Low", ROUTINE: "Routine", HIGH: "High", CRITICAL: "Critical" } as const;

export const RELIABILITY_LABEL = {
  A: "A · Reliable",
  B: "B · Usually reliable",
  C: "C · Fairly reliable",
  D: "D · Not usually reliable",
  E: "E · Unreliable",
} as const;
