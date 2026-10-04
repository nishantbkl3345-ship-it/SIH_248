"use client";

import { ChevronRight, CircleCheck, Info, TriangleAlert } from "lucide-react";

import type { ScenarioEditor } from "@/components/scenario/use-scenario-editor";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import type { Issue, ScenarioContent } from "@/lib/scenario/types";

/** Where an issue lives, in words: "Phase 2 · Report: Coastal road". */
function locate(content: ScenarioContent, issue: Issue): string {
  const phaseIndex = content.phases.findIndex((p) => p.id === issue.phaseId);
  if (phaseIndex < 0) return "Setup";
  const phase = content.phases[phaseIndex];
  const base = `Phase ${phaseIndex + 1}`;
  if (issue.entityId === phase.id || !issue.entityId) return `${base} · ${phase.title || "Untitled"}`;

  const report = phase.reports.find((r) => r.id === issue.entityId);
  if (report) return `${base} · Report: ${report.title || "untitled"}`;
  const rule = phase.rules.find((r) => r.id === issue.entityId);
  if (rule) return `${base} · Communication rule${rule.name ? `: ${rule.name}` : ""}`;
  const decision = phase.decisionPoints.find((d) => d.id === issue.entityId);
  if (decision) return `${base} · Decision: ${decision.question || "untitled"}`;
  return base;
}

function IssueList({
  issues,
  content,
  onGo,
  tone,
}: {
  issues: Issue[];
  content: ScenarioContent;
  onGo: (issue: Issue) => void;
  tone: "error" | "warning";
}) {
  const Icon = tone === "error" ? TriangleAlert : Info;
  return (
    <ul className="grid gap-1.5">
      {issues.map((issue, i) => (
        <li key={`${issue.path}-${i}`}>
          <button
            type="button"
            onClick={() => onGo(issue)}
            className="flex w-full items-start gap-3 rounded-lg border p-3 text-left transition-colors hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring"
          >
            <Icon
              className={`mt-0.5 size-4 shrink-0 ${tone === "error" ? "text-destructive" : "text-amber-600"}`}
              aria-hidden
            />
            <span className="min-w-0 flex-1">
              <span className="block text-sm font-medium">{issue.message}</span>
              <span className="block truncate text-xs text-muted-foreground">{locate(content, issue)}</span>
            </span>
            <ChevronRight className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden />
          </button>
        </li>
      ))}
    </ul>
  );
}

export function ReadinessTab({ editor, onGo }: { editor: ScenarioEditor; onGo: (issue: Issue) => void }) {
  const { issues, content, saveState } = editor;
  const errors = issues.filter((i) => i.severity === "error");
  const warnings = issues.filter((i) => i.severity === "warning");
  const stale = saveState !== "saved";

  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-6">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            {errors.length === 0 ? (
              <>
                <CircleCheck className="size-5 text-emerald-600" aria-hidden />
                Ready to publish
              </>
            ) : (
              <>
                <TriangleAlert className="size-5 text-destructive" aria-hidden />
                {errors.length} {errors.length === 1 ? "issue blocks" : "issues block"} publishing
              </>
            )}
          </CardTitle>
          <CardDescription>
            The scenario is checked each time it saves.
            {stale && " Showing the result of the last save; recent edits are not checked yet."}
          </CardDescription>
        </CardHeader>
        {errors.length > 0 && (
          <CardContent>
            <IssueList issues={errors} content={content} onGo={onGo} tone="error" />
          </CardContent>
        )}
      </Card>

      {warnings.length > 0 && (
        <Card>
          <CardHeader>
            <CardTitle>Worth a look</CardTitle>
            <CardDescription>These do not block publishing.</CardDescription>
          </CardHeader>
          <CardContent>
            <IssueList issues={warnings} content={content} onGo={onGo} tone="warning" />
          </CardContent>
        </Card>
      )}
    </div>
  );
}
