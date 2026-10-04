import { describe, expect, it } from "vitest";

import {
  addItem,
  addPhase,
  changeRuleMode,
  findItem,
  movePhase,
  newChannel,
  newDecisionPoint,
  newPhase,
  newReport,
  newRule,
  removeChannel,
  removeItem,
  updateItem,
} from "@/lib/scenario/edit";
import { formatClock, formatDuration, parseClock } from "@/lib/scenario/time";
import { buildTimeline, phaseStarts, visibleToTeam } from "@/lib/scenario/timeline";
import type { ScenarioContent } from "@/lib/scenario/types";

describe("clock", () => {
  it("formats simulation time", () => {
    expect(formatClock(0)).toBe("00:00");
    expect(formatClock(150)).toBe("02:30");
    expect(formatClock(3725)).toBe("1:02:05");
  });

  it("parses what instructors type", () => {
    expect(parseClock("2:30")).toBe(150);
    expect(parseClock("02:30")).toBe(150);
    expect(parseClock("5")).toBe(300);
    expect(parseClock("2.5")).toBe(150);
    expect(parseClock("90:00")).toBe(5400);
    expect(parseClock("1:02:05")).toBe(3725);
    expect(parseClock(" 0:45 ")).toBe(45);
  });

  it("rejects things that are not times", () => {
    for (const bad of ["", "abc", "2:75", "1:75:00", "-3", "1:2:3:4", "2:"]) {
      expect(parseClock(bad)).toBeNull();
    }
  });

  it("round-trips", () => {
    for (const sec of [0, 59, 60, 725, 3599, 3600, 7325]) {
      expect(parseClock(formatClock(sec))).toBe(sec);
    }
  });

  it("formats durations", () => {
    expect(formatDuration(30)).toBe("30 s");
    expect(formatDuration(150)).toBe("2 min 30 s");
    expect(formatDuration(3600)).toBe("1 h");
    expect(formatDuration(0)).toBe("0 s");
  });
});

/** The example from the brief, as a two-phase scenario. */
function example(): ScenarioContent {
  const feed = { ...newChannel(), name: "Field Feed" };
  const p1 = { ...newPhase(0), title: "Assessment", durationSec: 360 };
  const p2 = { ...newPhase(1), title: "Commitment", durationSec: 360 };
  let c: ScenarioContent = {
    title: "Exercise Tidewatch",
    summary: "",
    difficulty: "INTERMEDIATE",
    teamCount: 2,
    traineeCount: 8,
    objectives: [],
    channels: [feed],
    phases: [p1, p2],
  };
  c = addItem(c, p1.id, "report", { ...newReport(120, feed.id), title: "Report A", source: "K-7" });
  c = addItem(c, p1.id, "rule", { ...newRule("PARTIAL", 240, feed.id), durationSec: 60, targetTeam: 1 });
  c = addItem(c, p1.id, "rule", { ...newRule("DELAY", 300, feed.id), durationSec: 600 });
  c = addItem(c, p2.id, "rule", {
    ...newRule("CONFLICTING", 60, feed.id),
    conflict: { topic: "Inland pass", sourceA: "K-7", contentA: "Open", sourceB: "Net", contentB: "Blocked" },
  });
  c = addItem(c, p2.id, "decision", { ...newDecisionPoint(180), question: "Which route?", deadlineSec: 120 });
  return c;
}

describe("buildTimeline", () => {
  it("lays the scenario out in exercise time", () => {
    const rows = buildTimeline(example()).map((e) => `${formatClock(e.atSec)} ${e.label}`);
    expect(rows).toEqual([
      "00:00 Exercise begins",
      "02:00 Report A arrives",
      "04:00 Channel degraded: 50% of content gets through",
      "05:00 Channel restored",
      "05:00 Information delayed by 30 s",
      "06:00 Phase transition: Commitment",
      "06:00 Delay ends",
      "07:00 Conflicting reports: Inland pass",
      "09:00 Decision point opens",
      "11:00 Decision point closes",
      "12:00 Exercise ends",
    ]);
  });

  it("cuts a window off at the end of its phase", () => {
    const end = buildTimeline(example()).find((e) => e.label === "Delay ends");
    // Authored to last 600 s from 05:00, but the phase ends at 06:00.
    expect(end?.atSec).toBe(360);
    expect(end?.derived).toBe(true);
  });

  it("links derived entries to the item that implies them", () => {
    const c = example();
    const decision = c.phases[1].decisionPoints[0];
    const linked = buildTimeline(c).filter((e) => e.item?.id === decision.id);
    expect(linked.map((e) => e.kind)).toEqual(["decision-open", "decision-close"]);
  });

  it("moves later phases when an earlier one is resized", () => {
    const c = example();
    c.phases[0] = { ...c.phases[0], durationSec: 600 };
    expect(phaseStarts(c.phases)).toEqual([0, 600]);
    expect(buildTimeline(c).find((e) => e.kind === "conflict")?.atSec).toBe(660);
  });

  it("is empty for a scenario with no phases", () => {
    expect(buildTimeline({ ...example(), phases: [] })).toEqual([]);
  });

  it("filters by team", () => {
    const all = buildTimeline(example());
    const team2 = visibleToTeam(all, 2);
    expect(visibleToTeam(all, null)).toHaveLength(all.length);
    expect(team2.some((e) => e.kind === "partial")).toBe(false);
    expect(visibleToTeam(all, 1).some((e) => e.kind === "partial")).toBe(true);
    expect(team2.some((e) => e.kind === "report")).toBe(true);
  });
});

describe("edits", () => {
  it("updates, finds and removes items without mutating the original", () => {
    const c = example();
    const phase = c.phases[0];
    const report = phase.reports[0];

    const renamed = updateItem(c, phase.id, "report", report.id, { title: "Report A1" });
    expect(findItem(renamed, phase.id, "report", report.id)?.title).toBe("Report A1");
    expect(report.title).toBe("Report A");
    expect(renamed.phases[1]).toBe(c.phases[1]);

    const removed = removeItem(renamed, phase.id, "report", report.id);
    expect(removed.phases[0].reports).toHaveLength(0);
    expect(removed.phases[0].rules).toHaveLength(2);
  });

  it("reorders phases and ignores moves off the end", () => {
    const c = example();
    const [a, b] = c.phases;
    expect(movePhase(c, a.id, 1).phases.map((p) => p.id)).toEqual([b.id, a.id]);
    expect(movePhase(c, a.id, -1)).toBe(c);
    expect(movePhase(c, b.id, 1)).toBe(c);
    expect(addPhase(c).content.phases).toHaveLength(3);
  });

  it("clears references when a channel is removed", () => {
    const c = example();
    const out = removeChannel(c, c.channels[0].id);
    expect(out.channels).toHaveLength(0);
    expect(out.phases[0].reports[0].channelId).toBeNull();
    expect(out.phases.flatMap((p) => p.rules).every((r) => r.channelId === null)).toBe(true);
  });

  it("resets mode-specific settings when a rule changes mode", () => {
    const delay = { ...newRule("DELAY", 90, "ch"), name: "Lag", targetTeam: 2, durationSec: 45 };
    const conflict = changeRuleMode(delay, "CONFLICTING");
    expect(conflict).toMatchObject({ id: delay.id, name: "Lag", atSec: 90, targetTeam: 2, delaySec: 0, durationSec: 0 });
    expect(conflict.conflict).not.toBeNull();

    const partial = changeRuleMode(conflict, "PARTIAL");
    expect(partial).toMatchObject({ keepPercent: 50, conflict: null, durationSec: 60 });
    expect(changeRuleMode(partial, "PARTIAL")).toBe(partial);
  });
});
