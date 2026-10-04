"use client";

import { ArrowLeft, Check, CircleAlert, CloudOff, LoaderCircle, Lock, Rocket, Undo2 } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { ItemEditor } from "@/components/scenario/item-editor";
import { ScenarioPreview } from "@/components/scenario/preview";
import { ReadinessTab } from "@/components/scenario/readiness-tab";
import { SetupTab } from "@/components/scenario/setup-tab";
import { TimelineTab } from "@/components/scenario/timeline-tab";
import {
  useScenarioEditor,
  type SaveState,
  type ScenarioEditor,
} from "@/components/scenario/use-scenario-editor";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { formatClock } from "@/lib/scenario/time";
import { totalDuration } from "@/lib/scenario/timeline";
import type { Issue, ItemRef, ItemType, Scenario } from "@/lib/scenario/types";

const SAVE_LABEL: Record<SaveState, string> = {
  saved: "All changes saved",
  dirty: "Unsaved changes",
  saving: "Saving…",
  error: "Not saved",
  conflict: "Changed elsewhere",
};

function SaveIndicator({ editor }: { editor: ScenarioEditor }) {
  const { saveState, flush } = editor;
  const Icon =
    saveState === "saved" ? Check : saveState === "saving" ? LoaderCircle : saveState === "dirty" ? CircleAlert : CloudOff;
  const failed = saveState === "error" || saveState === "conflict";
  return (
    <span
      role="status"
      className={`flex items-center gap-1.5 text-xs ${failed ? "font-medium text-destructive" : "text-muted-foreground"}`}
    >
      <Icon className={`size-3.5 ${saveState === "saving" ? "animate-spin" : ""}`} aria-hidden />
      {SAVE_LABEL[saveState]}
      {saveState === "error" && (
        <Button type="button" variant="outline" size="xs" onClick={() => void flush()}>
          Retry
        </Button>
      )}
    </span>
  );
}

function PublishDialog({
  editor,
  open,
  onOpenChange,
}: {
  editor: ScenarioEditor;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [confirmed, setConfirmed] = useState(false);
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function publish() {
    setPending(true);
    setError(null);
    const failure = await editor.publish();
    setPending(false);
    if (failure) {
      setError(failure);
    } else {
      setConfirmed(false);
      onOpenChange(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Publish this scenario?</DialogTitle>
          <DialogDescription>
            Publishing makes the scenario available for exercises and locks it against editing. You can
            revert it to a draft as long as no exercise has used it.
          </DialogDescription>
        </DialogHeader>
        <label className="flex items-start gap-2.5 rounded-lg border p-3 text-sm">
          <input
            type="checkbox"
            className="mt-0.5 size-4 accent-foreground"
            checked={confirmed}
            onChange={(e) => setConfirmed(e.target.checked)}
          />
          <span>
            I confirm that every place, organisation, person and event in this scenario is fictional, and
            that it contains no real-world operational information.
          </span>
        </label>
        {error && (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={pending}>
            Cancel
          </Button>
          <Button type="button" onClick={publish} disabled={!confirmed || pending} aria-busy={pending}>
            {pending ? <LoaderCircle className="animate-spin" aria-hidden /> : <Rocket aria-hidden />}
            Publish scenario
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function itemTypeOf(editor: ScenarioEditor, issue: Issue): ItemType | null {
  const phase = editor.content.phases.find((p) => p.id === issue.phaseId);
  if (!phase || !issue.entityId) return null;
  if (phase.reports.some((r) => r.id === issue.entityId)) return "report";
  if (phase.rules.some((r) => r.id === issue.entityId)) return "rule";
  if (phase.decisionPoints.some((d) => d.id === issue.entityId)) return "decision";
  return null;
}

export function ScenarioBuilder({ initial }: { initial: Scenario }) {
  const editor = useScenarioEditor(initial);
  const { content, readOnly, issues, saveState, saveError } = editor;

  const [tab, setTab] = useState("timeline");
  const [phaseId, setPhaseId] = useState<string | undefined>(initial.phases[0]?.id);
  const [selected, setSelected] = useState<ItemRef | null>(null);
  const [publishOpen, setPublishOpen] = useState(false);
  const [revertError, setRevertError] = useState<string | null>(null);
  const [reverting, setReverting] = useState(false);

  const errorCount = issues.filter((i) => i.severity === "error").length;

  function goToIssue(issue: Issue) {
    if (!issue.phaseId) {
      setTab("setup");
      return;
    }
    setTab("timeline");
    setPhaseId(issue.phaseId);
    const type = itemTypeOf(editor, issue);
    if (type && issue.entityId) setSelected({ phaseId: issue.phaseId, type, id: issue.entityId });
  }

  async function revert() {
    setReverting(true);
    setRevertError(await editor.unpublish());
    setReverting(false);
  }

  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-6">
      <header className="grid gap-3">
        <Link
          href="/instructor/scenarios"
          className="flex w-fit items-center gap-1 text-sm text-muted-foreground hover:text-foreground"
        >
          <ArrowLeft className="size-4" aria-hidden />
          Scenario library
        </Link>
        <div className="flex flex-wrap items-start justify-between gap-x-6 gap-y-3">
          <div className="min-w-0 space-y-1.5">
            <h1 className="text-2xl font-semibold tracking-tight break-words">
              {content.title || "Untitled scenario"}
            </h1>
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
              <Badge variant={readOnly ? "default" : "secondary"}>
                {readOnly ? "Published" : "Draft"}
              </Badge>
              <span className="font-mono text-xs tabular-nums text-muted-foreground">
                {formatClock(totalDuration(content.phases))} · {content.phases.length}{" "}
                {content.phases.length === 1 ? "phase" : "phases"}
              </span>
              {!readOnly && <SaveIndicator editor={editor} />}
            </div>
          </div>
          {readOnly ? (
            <Button type="button" variant="outline" onClick={revert} disabled={reverting}>
              {reverting ? <LoaderCircle className="animate-spin" aria-hidden /> : <Undo2 aria-hidden />}
              Revert to draft
            </Button>
          ) : (
            <Button
              type="button"
              disabled={errorCount > 0 || saveState === "conflict"}
              title={errorCount > 0 ? "Resolve the readiness issues first" : undefined}
              onClick={() => setPublishOpen(true)}
            >
              <Rocket aria-hidden />
              Publish
            </Button>
          )}
        </div>
      </header>

      {saveState === "conflict" && (
        <Alert variant="destructive">
          <AlertTitle>This scenario was changed somewhere else</AlertTitle>
          <AlertDescription>
            Saving has stopped so that neither copy overwrites the other. Reload the page to get the latest
            version; edits made here since the last successful save will be lost.
          </AlertDescription>
        </Alert>
      )}
      {saveState === "error" && saveError && (
        <Alert variant="destructive">
          <AlertTitle>Your changes are not saved</AlertTitle>
          <AlertDescription>{saveError}</AlertDescription>
        </Alert>
      )}
      {revertError && (
        <Alert variant="destructive">
          <AlertDescription>{revertError}</AlertDescription>
        </Alert>
      )}

      {readOnly ? (
        <>
          <Alert>
            <Lock aria-hidden />
            <AlertTitle>Published and locked</AlertTitle>
            <AlertDescription>
              This is the scenario as it will run. Revert it to a draft to make changes.
            </AlertDescription>
          </Alert>
          <ScenarioPreview content={content} />
        </>
      ) : (
        <Tabs value={tab} onValueChange={(v) => setTab(String(v))}>
          <TabsList variant="line">
            <TabsTrigger value="setup">Setup</TabsTrigger>
            <TabsTrigger value="timeline">Timeline</TabsTrigger>
            <TabsTrigger value="readiness">
              Readiness
              {errorCount > 0 && <Badge variant="destructive">{errorCount}</Badge>}
            </TabsTrigger>
            <TabsTrigger value="preview">Preview</TabsTrigger>
          </TabsList>
          <TabsContent value="setup" className="min-w-0 pt-4">
            <SetupTab editor={editor} />
          </TabsContent>
          <TabsContent value="timeline" className="min-w-0 pt-4">
            <TimelineTab
              editor={editor}
              phaseId={phaseId}
              onSelectPhase={setPhaseId}
              onOpenItem={setSelected}
            />
          </TabsContent>
          <TabsContent value="readiness" className="min-w-0 pt-4">
            <ReadinessTab editor={editor} onGo={goToIssue} />
          </TabsContent>
          <TabsContent value="preview" className="min-w-0 pt-4">
            <ScenarioPreview content={content} />
          </TabsContent>
        </Tabs>
      )}

      {!readOnly && (
        <>
          <ItemEditor editor={editor} selected={selected} onClose={() => setSelected(null)} />
          <PublishDialog editor={editor} open={publishOpen} onOpenChange={setPublishOpen} />
        </>
      )}
    </div>
  );
}
