"use client";

import { Clock, Layers, LoaderCircle, Plus, Users } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { ConfirmButton } from "@/components/scenario/confirm-button";
import { DIFFICULTY_LABEL } from "@/components/scenario/kinds";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { apiFetch } from "@/lib/api/client";
import { ApiError } from "@/lib/api/errors";
import { formatDuration } from "@/lib/scenario/time";
import type { Scenario, ScenarioSummary } from "@/lib/scenario/types";

const message = (err: unknown) =>
  err instanceof ApiError ? err.message : "Something went wrong. Please try again.";

export function NewScenarioButton() {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function create(event: React.FormEvent) {
    event.preventDefault();
    if (pending) return;
    if (!title.trim()) {
      setError("Give the scenario a name.");
      return;
    }
    setPending(true);
    setError(null);
    try {
      const { scenario } = await apiFetch<{ scenario: Scenario }>("/scenarios", {
        body: { title: title.trim() },
      });
      router.push(`/instructor/scenarios/${scenario.id}`);
      // Stay pending: the page is navigating to the builder.
    } catch (err) {
      setPending(false);
      setError(message(err));
    }
  }

  return (
    <>
      <Button type="button" onClick={() => setOpen(true)}>
        <Plus aria-hidden />
        New scenario
      </Button>
      <Dialog open={open} onOpenChange={(next) => !pending && setOpen(next)}>
        <DialogContent className="sm:max-w-md">
          <form onSubmit={create} className="grid gap-4">
            <DialogHeader>
              <DialogTitle>New scenario</DialogTitle>
              <DialogDescription>
                Starts as a draft with one phase and three channels. Use fictional names throughout.
              </DialogDescription>
            </DialogHeader>
            <div className="grid gap-1.5">
              <Label htmlFor="new-scenario-title">Scenario name</Label>
              <Input
                id="new-scenario-title"
                value={title}
                maxLength={120}
                autoFocus
                placeholder="e.g. Exercise Tidewatch"
                aria-invalid={error ? true : undefined}
                onChange={(e) => setTitle(e.target.value)}
              />
              {error && <p className="text-xs text-destructive">{error}</p>}
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setOpen(false)} disabled={pending}>
                Cancel
              </Button>
              <Button type="submit" disabled={pending} aria-busy={pending}>
                {pending && <LoaderCircle className="animate-spin" aria-hidden />}
                Create and open
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </>
  );
}

const updated = new Intl.DateTimeFormat("en-GB", { dateStyle: "medium", timeStyle: "short", timeZone: "UTC" });

export function ScenarioLibrary({ scenarios }: { scenarios: ScenarioSummary[] }) {
  const router = useRouter();
  const [error, setError] = useState<string | null>(null);

  async function remove(id: string) {
    setError(null);
    try {
      await apiFetch<void>(`/scenarios/${id}`, { method: "DELETE" });
      router.refresh();
    } catch (err) {
      setError(message(err));
    }
  }

  if (scenarios.length === 0) {
    return (
      <Card>
        <CardContent className="flex flex-col items-center gap-3 py-14 text-center">
          <span className="flex size-10 items-center justify-center rounded-full bg-secondary">
            <Layers className="size-5" aria-hidden />
          </span>
          <h2 className="text-base font-semibold">No scenarios yet</h2>
          <p className="max-w-sm text-sm text-muted-foreground">
            A scenario is the script for an exercise: phases, the reports trainees receive, when
            communications degrade, and the decisions they must make.
          </p>
          <NewScenarioButton />
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="grid gap-4">
      {error && (
        <Alert variant="destructive">
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      )}
      <ul className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {scenarios.map((s) => (
          <li key={s.id}>
            <Card className="h-full">
              <CardHeader>
                <div className="flex items-center gap-2">
                  <Badge variant={s.status === "PUBLISHED" ? "default" : "secondary"}>
                    {s.status === "PUBLISHED" ? "Published" : "Draft"}
                  </Badge>
                  <Badge variant="outline">{DIFFICULTY_LABEL[s.difficulty]}</Badge>
                </div>
                <CardTitle className="text-base">
                  <Link
                    href={`/instructor/scenarios/${s.id}`}
                    className="underline-offset-4 hover:underline focus-visible:underline focus-visible:outline-none"
                  >
                    {s.title || "Untitled scenario"}
                  </Link>
                </CardTitle>
                <CardDescription className="line-clamp-2 min-h-10">
                  {s.summary || "No description yet."}
                </CardDescription>
              </CardHeader>
              <CardContent className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                <span className="flex items-center gap-1.5">
                  <Clock className="size-3.5" aria-hidden />
                  {formatDuration(s.estDurationSec)}
                </span>
                <span className="flex items-center gap-1.5">
                  <Layers className="size-3.5" aria-hidden />
                  {s.phaseCount} {s.phaseCount === 1 ? "phase" : "phases"}
                </span>
                <span className="flex items-center gap-1.5">
                  <Users className="size-3.5" aria-hidden />
                  {s.teamCount} {s.teamCount === 1 ? "team" : "teams"} · {s.traineeCount} trainees
                </span>
              </CardContent>
              <CardFooter className="mt-auto justify-between gap-2">
                <span className="text-xs text-muted-foreground">
                  Updated {updated.format(new Date(s.updatedAt))} UTC
                </span>
                <ConfirmButton iconOnly label={`Delete ${s.title}`} onConfirm={() => remove(s.id)} />
              </CardFooter>
            </Card>
          </li>
        ))}
      </ul>
    </div>
  );
}
