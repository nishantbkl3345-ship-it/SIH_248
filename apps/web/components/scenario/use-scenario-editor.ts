"use client";

import { useCallback, useEffect, useRef, useState } from "react";

import { apiFetch } from "@/lib/api/client";
import { ApiError } from "@/lib/api/errors";
import { pickContent, type Issue, type Scenario, type ScenarioContent, type ScenarioStatus } from "@/lib/scenario/types";

export type SaveState = "saved" | "dirty" | "saving" | "error" | "conflict";

const AUTOSAVE_DELAY_MS = 1200;

interface Meta {
  status: ScenarioStatus;
  issues: Issue[];
  updatedAt: string;
  publishedAt: string | null;
}

const metaOf = (s: Scenario): Meta => ({
  status: s.status,
  issues: s.issues,
  updatedAt: s.updatedAt,
  publishedAt: s.publishedAt,
});

function describe(err: unknown): string {
  if (err instanceof ApiError) {
    const [path, detail] = Object.entries(err.details)[0] ?? [];
    return path ? `${err.message} (${path} ${detail})` : err.message;
  }
  return "Something went wrong. Please try again.";
}

/**
 * Holds the scenario being edited and keeps the server copy up to date.
 *
 * Edits are applied locally at once and saved after a short pause. Saves are
 * serialised, and each carries the revision the server last confirmed, so a
 * copy edited elsewhere is detected ("conflict") instead of overwritten.
 * The server's reply updates issues and revision only; it never replaces
 * local content, because the user may have kept typing while it was in flight.
 */
export function useScenarioEditor(initial: Scenario) {
  const [content, setContent] = useState<ScenarioContent>(() => pickContent(initial));
  const [meta, setMeta] = useState<Meta>(() => metaOf(initial));
  const [saveState, setSaveState] = useState<SaveState>("saved");
  const [saveError, setSaveError] = useState<string | null>(null);

  const id = initial.id;
  const contentRef = useRef(content);
  const revisionRef = useRef(initial.revision);
  const versionRef = useRef(0); // bumped on every local edit
  const savedVersionRef = useRef(0); // version the server has
  const inFlightRef = useRef<Promise<boolean> | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const stoppedRef = useRef(initial.status !== "DRAFT"); // conflict or not a draft: stop saving

  /** Saves pending edits now. Resolves true once the server has everything. */
  const flush = useCallback(async (): Promise<boolean> => {
    if (timerRef.current) clearTimeout(timerRef.current);
    timerRef.current = null;

    for (;;) {
      while (inFlightRef.current) await inFlightRef.current;
      if (stoppedRef.current) return savedVersionRef.current === versionRef.current;
      if (savedVersionRef.current === versionRef.current) return true;

      const version = versionRef.current;
      const run = (async () => {
        setSaveState("saving");
        try {
          const { scenario } = await apiFetch<{ scenario: Scenario }>(`/scenarios/${id}`, {
            method: "PUT",
            body: { revision: revisionRef.current, ...contentRef.current },
          });
          revisionRef.current = scenario.revision;
          savedVersionRef.current = version;
          setMeta(metaOf(scenario));
          setSaveError(null);
          setSaveState(versionRef.current === version ? "saved" : "dirty");
          return true;
        } catch (err) {
          if (err instanceof ApiError && err.status === 409) {
            stoppedRef.current = true;
            setSaveState("conflict");
          } else {
            setSaveState("error");
          }
          setSaveError(describe(err));
          return false;
        }
      })();
      inFlightRef.current = run;
      const ok = await run;
      inFlightRef.current = null;
      if (!ok) return false;
      // Loop: edits made while that save was in flight still need saving.
    }
  }, [id]);

  const update = useCallback(
    (change: (c: ScenarioContent) => ScenarioContent) => {
      if (stoppedRef.current) return;
      const next = change(contentRef.current);
      if (next === contentRef.current) return;
      contentRef.current = next;
      versionRef.current += 1;
      setContent(next);
      setSaveState("dirty");
      if (timerRef.current) clearTimeout(timerRef.current);
      timerRef.current = setTimeout(() => void flush(), AUTOSAVE_DELAY_MS);
    },
    [flush],
  );

  /** Runs a status change (publish / revert) after saving pending edits. */
  const transition = useCallback(
    async (action: "publish" | "unpublish", body?: unknown): Promise<string | null> => {
      if (action === "publish" && !(await flush())) {
        return "Your latest changes could not be saved, so the scenario was not published.";
      }
      try {
        const { scenario } = await apiFetch<{ scenario: Scenario }>(`/scenarios/${id}/${action}`, {
          method: "POST",
          body,
        });
        revisionRef.current = scenario.revision;
        stoppedRef.current = scenario.status !== "DRAFT";
        setMeta(metaOf(scenario));
        setSaveState("saved");
        setSaveError(null);
        return null;
      } catch (err) {
        return describe(err);
      }
    },
    [flush, id],
  );

  const publish = useCallback(() => transition("publish", { confirmFictional: true }), [transition]);
  const unpublish = useCallback(() => transition("unpublish"), [transition]);

  // Leaving the page: save what is pending, and warn if the tab is closing
  // before that can finish.
  const unsaved = saveState === "dirty" || saveState === "saving" || saveState === "error";
  useEffect(() => {
    if (!unsaved) return;
    const warn = (e: BeforeUnloadEvent) => e.preventDefault();
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, [unsaved]);

  useEffect(() => () => void flush(), [flush]);

  return {
    content,
    update,
    flush,
    publish,
    unpublish,
    saveState,
    saveError,
    readOnly: meta.status !== "DRAFT",
    ...meta,
  };
}

export type ScenarioEditor = ReturnType<typeof useScenarioEditor>;
