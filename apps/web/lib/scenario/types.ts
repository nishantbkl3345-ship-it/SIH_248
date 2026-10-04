// Mirrors the scenario document exchanged with the API
// (services/api/internal/transport/httpapi/handlers_scenario.go).
// Every atSec is seconds from the start of the phase that owns the item.

export type Difficulty = "BASIC" | "INTERMEDIATE" | "ADVANCED";
export type ScenarioStatus = "DRAFT" | "PUBLISHED" | "ARCHIVED";
export type ChannelKind = "TEAM" | "CROSS_TEAM" | "BROADCAST" | "FEED";
export type Priority = "LOW" | "ROUTINE" | "HIGH" | "CRITICAL";
export type Reliability = "A" | "B" | "C" | "D" | "E";
export type DegradationMode = "NORMAL" | "DELAY" | "DROPOUT" | "PARTIAL" | "CONFLICTING";
export type DecisionScope = "INDIVIDUAL" | "TEAM";

export interface Channel {
  id: string;
  name: string;
  kind: ChannelKind;
  baseLatencySec: number;
}

export interface Report {
  id: string;
  title: string;
  source: string;
  content: string;
  atSec: number;
  priority: Priority;
  reliability: Reliability;
  /** null addresses every team. */
  targetTeam: number | null;
  channelId: string | null;
}

export interface Conflict {
  topic: string;
  sourceA: string;
  contentA: string;
  sourceB: string;
  contentB: string;
}

/** A communication rule in force for a window of its phase. */
export interface Rule {
  id: string;
  name: string;
  mode: DegradationMode;
  atSec: number;
  durationSec: number;
  /** null applies to every channel. */
  channelId: string | null;
  /** null applies to every team. */
  targetTeam: number | null;
  delaySec: number;
  keepPercent: number;
  conflict: Conflict | null;
}

export interface DecisionOption {
  id: string;
  label: string;
  description: string;
}

export interface DecisionPoint {
  id: string;
  question: string;
  scope: DecisionScope;
  atSec: number;
  /** Seconds after opening at which the decision closes. */
  deadlineSec: number;
  rationaleRequired: boolean;
  evaluationCriteria: string;
  options: DecisionOption[];
}

export interface Phase {
  id: string;
  title: string;
  description: string;
  durationSec: number;
  reports: Report[];
  rules: Rule[];
  decisionPoints: DecisionPoint[];
}

/** The part of a scenario an instructor edits. */
export interface ScenarioContent {
  title: string;
  summary: string;
  difficulty: Difficulty;
  teamCount: number;
  traineeCount: number;
  objectives: string[];
  channels: Channel[];
  phases: Phase[];
}

export interface Issue {
  severity: "error" | "warning";
  path: string;
  message: string;
  phaseId: string | null;
  entityId: string | null;
}

export interface Scenario extends ScenarioContent {
  id: string;
  status: ScenarioStatus;
  revision: number;
  estDurationSec: number;
  createdAt: string;
  updatedAt: string;
  publishedAt: string | null;
  issues: Issue[];
}

export interface ScenarioSummary {
  id: string;
  title: string;
  summary: string;
  difficulty: Difficulty;
  status: ScenarioStatus;
  teamCount: number;
  traineeCount: number;
  phaseCount: number;
  estDurationSec: number;
  updatedAt: string;
  publishedAt: string | null;
}

export type ItemType = "report" | "rule" | "decision";

export interface ItemRef {
  phaseId: string;
  type: ItemType;
  id: string;
}

export function pickContent(s: Scenario): ScenarioContent {
  return {
    title: s.title,
    summary: s.summary,
    difficulty: s.difficulty,
    teamCount: s.teamCount,
    traineeCount: s.traineeCount,
    objectives: s.objectives,
    channels: s.channels,
    phases: s.phases,
  };
}
