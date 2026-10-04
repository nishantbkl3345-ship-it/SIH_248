"use client";

import { useState } from "react";

import {
  CHANNEL_KIND_LABEL,
  DIFFICULTY_LABEL,
  PRIORITY_LABEL,
  RELIABILITY_LABEL,
} from "@/components/scenario/kinds";
import { MasterTimeline, TimelineLegend } from "@/components/scenario/master-timeline";
import { TimelineList } from "@/components/scenario/timeline-list";
import { Badge } from "@/components/ui/badge";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { findItem } from "@/lib/scenario/edit";
import { formatClock, formatDuration } from "@/lib/scenario/time";
import {
  buildTimeline,
  channelName,
  phaseStarts,
  teamLabel,
  totalDuration,
  visibleToTeam,
  type TimelineEntry,
} from "@/lib/scenario/timeline";
import type { ScenarioContent } from "@/lib/scenario/types";

function Fact({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div>
      <dt className="text-xs font-medium text-muted-foreground">{label}</dt>
      <dd className="text-sm font-medium">{children}</dd>
    </div>
  );
}

function Quote({ source, children }: { source: string; children: React.ReactNode }) {
  return (
    <figure className="rounded-lg border bg-muted/30 p-3">
      <figcaption className="text-xs font-medium text-muted-foreground">{source}</figcaption>
      <blockquote className="mt-1 text-sm whitespace-pre-wrap">{children || "—"}</blockquote>
    </figure>
  );
}

/** The content behind a timeline entry, as it would be used in the exercise. */
function Detail({ entry, content }: { entry: TimelineEntry; content: ScenarioContent }) {
  if (!entry.item || entry.derived) return null;

  if (entry.item.type === "report") {
    const r = findItem(content, entry.phaseId, "report", entry.item.id);
    if (!r) return null;
    return (
      <div className="grid gap-2">
        <Quote source={`${r.source || "No source"} · reliability ${RELIABILITY_LABEL[r.reliability]}`}>
          {r.content}
        </Quote>
        <p className="text-xs text-muted-foreground">
          {PRIORITY_LABEL[r.priority]} priority · to {teamLabel(r.targetTeam)} · via{" "}
          {channelName(content.channels, r.channelId, "no channel")}
        </p>
      </div>
    );
  }

  if (entry.item.type === "rule") {
    const r = findItem(content, entry.phaseId, "rule", entry.item.id);
    if (!r || r.mode !== "CONFLICTING" || !r.conflict) return null;
    return (
      <div className="grid gap-2 sm:grid-cols-2">
        <Quote source={r.conflict.sourceA || "Source A"}>{r.conflict.contentA}</Quote>
        <Quote source={r.conflict.sourceB || "Source B"}>{r.conflict.contentB}</Quote>
      </div>
    );
  }

  const d = findItem(content, entry.phaseId, "decision", entry.item.id);
  if (!d) return null;
  return (
    <div className="grid gap-2 rounded-lg border p-3">
      <ol className="grid gap-1 text-sm">
        {d.options.map((o, i) => (
          <li key={o.id} className="flex gap-2">
            <span className="font-mono text-xs text-muted-foreground">{String.fromCharCode(65 + i)}</span>
            {o.label || "—"}
          </li>
        ))}
      </ol>
      <p className="text-xs text-muted-foreground">
        {d.scope === "TEAM" ? "Decided by each team" : "Decided by each trainee"} ·{" "}
        {formatDuration(d.deadlineSec)} to decide · rationale {d.rationaleRequired ? "required" : "optional"}
      </p>
      {d.evaluationCriteria.trim() && (
        <p className="border-t pt-2 text-xs">
          <span className="font-medium">Evaluation criteria (instructor only): </span>
          <span className="whitespace-pre-wrap text-muted-foreground">{d.evaluationCriteria}</span>
        </p>
      )}
    </div>
  );
}

/**
 * Read-only run sheet: what the exercise is, then everything that happens in
 * order. "View as" narrows it to what one team is subject to.
 */
export function ScenarioPreview({ content }: { content: ScenarioContent }) {
  const [team, setTeam] = useState<number | null>(null);
  const viewTeam = team !== null && team <= content.teamCount ? team : null;

  const starts = phaseStarts(content.phases);
  const entries = visibleToTeam(buildTimeline(content), viewTeam);
  const objectives = content.objectives.filter((o) => o.trim());

  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-6">
      <Card>
        <CardHeader>
          <CardDescription>Scenario briefing</CardDescription>
          <CardTitle className="text-xl">{content.title || "Untitled scenario"}</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-5">
          <p className="max-w-3xl text-sm whitespace-pre-wrap text-muted-foreground">
            {content.summary || "No description yet."}
          </p>
          <dl className="grid grid-cols-2 gap-4 sm:grid-cols-4">
            <Fact label="Difficulty">{DIFFICULTY_LABEL[content.difficulty]}</Fact>
            <Fact label="Estimated duration">{formatDuration(totalDuration(content.phases))}</Fact>
            <Fact label="Teams">{content.teamCount}</Fact>
            <Fact label="Trainees">{content.traineeCount}</Fact>
          </dl>
          <div className="grid gap-5 sm:grid-cols-2">
            <div>
              <h3 className="mb-1.5 text-xs font-medium text-muted-foreground">Learning objectives</h3>
              {objectives.length ? (
                <ol className="grid list-decimal gap-1 pl-5 text-sm">
                  {objectives.map((o, i) => (
                    <li key={i}>{o}</li>
                  ))}
                </ol>
              ) : (
                <p className="text-sm text-muted-foreground">None yet.</p>
              )}
            </div>
            <div>
              <h3 className="mb-1.5 text-xs font-medium text-muted-foreground">Communication channels</h3>
              {content.channels.length ? (
                <ul className="grid gap-1 text-sm">
                  {content.channels.map((c) => (
                    <li key={c.id} className="flex flex-wrap items-center gap-2">
                      {c.name || "Unnamed channel"}
                      <Badge variant="outline">{CHANNEL_KIND_LABEL[c.kind]}</Badge>
                    </li>
                  ))}
                </ul>
              ) : (
                <p className="text-sm text-muted-foreground">None yet.</p>
              )}
            </div>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Scenario timeline</CardTitle>
          <CardDescription>
            {viewTeam === null
              ? "Everything that happens, for every team."
              : `Only what Team ${viewTeam} is subject to.`}
          </CardDescription>
          <CardAction>
            <div role="group" aria-label="View as" className="flex flex-wrap gap-1">
              {[null, ...Array.from({ length: content.teamCount }, (_, i) => i + 1)].map((t) => (
                <button
                  key={t ?? "all"}
                  type="button"
                  aria-pressed={viewTeam === t}
                  onClick={() => setTeam(t)}
                  className={cn(
                    "rounded-md border px-2 py-1 text-xs font-medium transition-colors focus-visible:outline-2 focus-visible:outline-ring",
                    viewTeam === t ? "border-foreground bg-foreground text-background" : "hover:bg-accent",
                  )}
                >
                  {t === null ? "All teams" : `Team ${t}`}
                </button>
              ))}
            </div>
          </CardAction>
        </CardHeader>
        <CardContent className="grid gap-6">
          <MasterTimeline content={content} />
          <TimelineLegend />

          {content.phases.map((p, i) => {
            const rows = entries.filter((e) => e.phaseId === p.id && e.kind !== "end");
            return (
              <section key={p.id} className="grid gap-3 border-t pt-5">
                <header>
                  <p className="font-mono text-xs tabular-nums text-muted-foreground">
                    {formatClock(starts[i])}–{formatClock(starts[i] + p.durationSec)} · Phase {i + 1}
                  </p>
                  <h3 className="text-base font-semibold">{p.title || "Untitled phase"}</h3>
                  {p.description.trim() && (
                    <p className="mt-1 max-w-3xl text-sm whitespace-pre-wrap text-muted-foreground">
                      {p.description}
                    </p>
                  )}
                </header>
                <TimelineList entries={rows} renderExtra={(e) => <Detail entry={e} content={content} />} />
              </section>
            );
          })}

          {entries.length > 0 && (
            <section className="border-t pt-5">
              <TimelineList entries={entries.filter((e) => e.kind === "end")} />
            </section>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
