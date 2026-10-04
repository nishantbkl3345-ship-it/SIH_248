import type {
  Channel,
  DecisionPoint,
  DegradationMode,
  ItemType,
  Phase,
  Report,
  Rule,
  ScenarioContent,
} from "@/lib/scenario/types";

// Pure, immutable edits to a scenario document. New items get their ids here,
// so references between items are stable before the first save.

export const LIMITS = {
  phases: 20,
  reports: 100,
  rules: 50,
  decisionPoints: 30,
  options: 8,
  channels: 12,
  objectives: 12,
  teams: 8,
  trainees: 60,
} as const;

const uid = () => crypto.randomUUID();

const COLLECTION = {
  report: "reports",
  rule: "rules",
  decision: "decisionPoints",
} as const satisfies Record<ItemType, keyof Phase>;

type ItemOf<T extends ItemType> = T extends "report" ? Report : T extends "rule" ? Rule : DecisionPoint;

export function newChannel(): Channel {
  return { id: uid(), name: "", kind: "TEAM", baseLatencySec: 0 };
}

export function newPhase(index: number): Phase {
  return {
    id: uid(),
    title: `Phase ${index + 1}`,
    description: "",
    durationSec: 300,
    reports: [],
    rules: [],
    decisionPoints: [],
  };
}

export function newReport(atSec: number, channelId: string | null): Report {
  return {
    id: uid(),
    title: "",
    source: "",
    content: "",
    atSec,
    priority: "ROUTINE",
    reliability: "B",
    targetTeam: null,
    channelId,
  };
}

export function newRule(mode: DegradationMode, atSec: number, channelId: string | null): Rule {
  return {
    id: uid(),
    name: "",
    mode,
    atSec,
    durationSec: mode === "CONFLICTING" ? 0 : 60,
    channelId,
    targetTeam: null,
    delaySec: mode === "DELAY" ? 30 : 0,
    keepPercent: mode === "PARTIAL" ? 50 : 0,
    conflict:
      mode === "CONFLICTING"
        ? { topic: "", sourceA: "", contentA: "", sourceB: "", contentB: "" }
        : null,
  };
}

export function newDecisionPoint(atSec: number): DecisionPoint {
  return {
    id: uid(),
    question: "",
    scope: "TEAM",
    atSec,
    deadlineSec: 120,
    rationaleRequired: true,
    evaluationCriteria: "",
    options: [
      { id: uid(), label: "", description: "" },
      { id: uid(), label: "", description: "" },
    ],
  };
}

export function newOption() {
  return { id: uid(), label: "", description: "" };
}

/** Switching mode keeps timing and scope but resets mode-specific settings. */
export function changeRuleMode(rule: Rule, mode: DegradationMode): Rule {
  if (rule.mode === mode) return rule;
  const fresh = newRule(mode, rule.atSec, rule.channelId);
  return {
    ...fresh,
    id: rule.id,
    name: rule.name,
    targetTeam: rule.targetTeam,
    durationSec: mode === "CONFLICTING" ? 0 : rule.durationSec || fresh.durationSec,
  };
}

export function updatePhase(c: ScenarioContent, phaseId: string, patch: Partial<Phase>): ScenarioContent {
  return { ...c, phases: c.phases.map((p) => (p.id === phaseId ? { ...p, ...patch } : p)) };
}

export function addPhase(c: ScenarioContent): { content: ScenarioContent; phase: Phase } {
  const phase = newPhase(c.phases.length);
  return { content: { ...c, phases: [...c.phases, phase] }, phase };
}

export function removePhase(c: ScenarioContent, phaseId: string): ScenarioContent {
  return { ...c, phases: c.phases.filter((p) => p.id !== phaseId) };
}

export function movePhase(c: ScenarioContent, phaseId: string, direction: -1 | 1): ScenarioContent {
  const from = c.phases.findIndex((p) => p.id === phaseId);
  const to = from + direction;
  if (from < 0 || to < 0 || to >= c.phases.length) return c;
  const phases = [...c.phases];
  [phases[from], phases[to]] = [phases[to], phases[from]];
  return { ...c, phases };
}

export function addItem<T extends ItemType>(
  c: ScenarioContent,
  phaseId: string,
  type: T,
  item: ItemOf<T>,
): ScenarioContent {
  const key = COLLECTION[type];
  return {
    ...c,
    phases: c.phases.map((p) => (p.id === phaseId ? { ...p, [key]: [...p[key], item] } : p)),
  };
}

export function updateItem<T extends ItemType>(
  c: ScenarioContent,
  phaseId: string,
  type: T,
  id: string,
  change: Partial<ItemOf<T>> | ((item: ItemOf<T>) => ItemOf<T>),
): ScenarioContent {
  const key = COLLECTION[type];
  return {
    ...c,
    phases: c.phases.map((p) => {
      if (p.id !== phaseId) return p;
      const items = (p[key] as ItemOf<T>[]).map((it) =>
        it.id !== id ? it : typeof change === "function" ? change(it) : { ...it, ...change },
      );
      return { ...p, [key]: items };
    }),
  };
}

export function removeItem(c: ScenarioContent, phaseId: string, type: ItemType, id: string): ScenarioContent {
  const key = COLLECTION[type];
  return {
    ...c,
    phases: c.phases.map((p) =>
      p.id === phaseId ? { ...p, [key]: (p[key] as { id: string }[]).filter((it) => it.id !== id) } : p,
    ),
  };
}

export function findItem<T extends ItemType>(
  c: ScenarioContent,
  phaseId: string,
  type: T,
  id: string,
): ItemOf<T> | undefined {
  const phase = c.phases.find((p) => p.id === phaseId);
  return (phase?.[COLLECTION[type]] as ItemOf<T>[] | undefined)?.find((it) => it.id === id);
}

/** Removing a channel also clears every reference to it. */
export function removeChannel(c: ScenarioContent, channelId: string): ScenarioContent {
  const clear = <T extends { channelId: string | null }>(it: T): T =>
    it.channelId === channelId ? { ...it, channelId: null } : it;
  return {
    ...c,
    channels: c.channels.filter((ch) => ch.id !== channelId),
    phases: c.phases.map((p) => ({ ...p, reports: p.reports.map(clear), rules: p.rules.map(clear) })),
  };
}
