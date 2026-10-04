"use client";

import { Plus, X } from "lucide-react";

import { ConfirmButton } from "@/components/scenario/confirm-button";
import {
  NumberField,
  SelectField,
  TextAreaField,
  TextField,
  selectClass,
} from "@/components/scenario/fields";
import { CHANNEL_KIND_LABEL, DIFFICULTY_LABEL } from "@/components/scenario/kinds";
import type { ScenarioEditor } from "@/components/scenario/use-scenario-editor";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { LIMITS, newChannel, removeChannel } from "@/lib/scenario/edit";
import { formatClock, formatDuration } from "@/lib/scenario/time";
import { totalDuration } from "@/lib/scenario/timeline";
import type { ChannelKind, Difficulty, Issue } from "@/lib/scenario/types";

const options = <T extends string>(labels: Record<T, string>) =>
  (Object.keys(labels) as T[]).map((value) => ({ value, label: labels[value] }));

export function SetupTab({ editor }: { editor: ScenarioEditor }) {
  const { content, update, issues } = editor;
  const issue = (path: string) => issues.find((i: Issue) => i.path === path)?.message;
  const duration = totalDuration(content.phases);

  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-6 lg:grid-cols-2">
      <Card className="lg:row-span-2">
        <CardHeader>
          <CardTitle>Scenario</CardTitle>
          <CardDescription>
            What the exercise is about. Keep every place, organisation and event fictional.
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4">
          <TextField
            label="Scenario name"
            value={content.title}
            onChange={(title) => update((c) => ({ ...c, title }))}
            maxLength={120}
            error={issue("title")}
          />
          <TextAreaField
            label="Description"
            value={content.summary}
            onChange={(summary) => update((c) => ({ ...c, summary }))}
            maxLength={2000}
            rows={5}
            placeholder="The fictional situation, and what trainees are asked to do in it."
            error={issue("summary")}
          />
          <div className="grid gap-4 sm:grid-cols-3">
            <SelectField<Difficulty>
              label="Difficulty"
              value={content.difficulty}
              onChange={(difficulty) => update((c) => ({ ...c, difficulty }))}
              options={options(DIFFICULTY_LABEL)}
            />
            <NumberField
              label="Teams"
              value={content.teamCount}
              min={1}
              max={LIMITS.teams}
              onChange={(teamCount) => update((c) => ({ ...c, teamCount }))}
              hint={`1 to ${LIMITS.teams}`}
            />
            <NumberField
              label="Trainees"
              value={content.traineeCount}
              min={1}
              max={LIMITS.trainees}
              onChange={(traineeCount) => update((c) => ({ ...c, traineeCount }))}
              hint="Across all teams"
              error={issue("traineeCount")}
            />
          </div>
          <div className="rounded-lg border bg-muted/40 px-3 py-2.5">
            <p className="text-xs font-medium text-muted-foreground">Estimated duration</p>
            <p className="font-mono text-lg tabular-nums">{formatClock(duration)}</p>
            <p className="text-xs text-muted-foreground">
              {formatDuration(duration)} across {content.phases.length}{" "}
              {content.phases.length === 1 ? "phase" : "phases"}. Change it by editing phase durations on
              the Timeline tab.
            </p>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Learning objectives</CardTitle>
          <CardDescription>What trainees should be better at afterwards.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-2">
          {content.objectives.map((objective, i) => (
            <div key={i} className="flex items-center gap-2">
              <span className="w-5 text-right font-mono text-xs text-muted-foreground">{i + 1}</span>
              <Input
                aria-label={`Objective ${i + 1}`}
                value={objective}
                maxLength={300}
                aria-invalid={issue(`objectives[${i}]`) ? true : undefined}
                onChange={(e) =>
                  update((c) => ({
                    ...c,
                    objectives: c.objectives.map((o, j) => (j === i ? e.target.value : o)),
                  }))
                }
              />
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                onClick={() => update((c) => ({ ...c, objectives: c.objectives.filter((_, j) => j !== i) }))}
              >
                <X aria-hidden />
                <span className="sr-only">Remove objective {i + 1}</span>
              </Button>
            </div>
          ))}
          {issue("objectives") && <p className="text-xs text-destructive">{issue("objectives")}</p>}
          <div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={content.objectives.length >= LIMITS.objectives}
              onClick={() => update((c) => ({ ...c, objectives: [...c.objectives, ""] }))}
            >
              <Plus aria-hidden />
              Add objective
            </Button>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Communication channels</CardTitle>
          <CardDescription>
            Where reports and messages travel. Degradation is applied per channel.
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-2">
          {content.channels.map((ch, i) => (
            <div key={ch.id} className="grid grid-cols-[1fr_auto] items-start gap-2 sm:grid-cols-[1fr_12rem_auto]">
              <Input
                aria-label={`Channel ${i + 1} name`}
                placeholder="Channel name"
                value={ch.name}
                maxLength={60}
                aria-invalid={issue(`channels[${i}].name`) ? true : undefined}
                onChange={(e) =>
                  update((c) => ({
                    ...c,
                    channels: c.channels.map((x) => (x.id === ch.id ? { ...x, name: e.target.value } : x)),
                  }))
                }
              />
              <select
                aria-label={`Channel ${i + 1} type`}
                className={`${selectClass} col-start-1 row-start-2 sm:col-start-2 sm:row-start-1`}
                value={ch.kind}
                onChange={(e) =>
                  update((c) => ({
                    ...c,
                    channels: c.channels.map((x) =>
                      x.id === ch.id ? { ...x, kind: e.target.value as ChannelKind } : x,
                    ),
                  }))
                }
              >
                {options(CHANNEL_KIND_LABEL).map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </select>
              <ConfirmButton
                iconOnly
                label={`Remove channel ${ch.name || i + 1}`}
                confirmLabel="Remove"
                onConfirm={() => update((c) => removeChannel(c, ch.id))}
              />
              {issue(`channels[${i}].name`) && (
                <p className="col-span-full text-xs text-destructive">{issue(`channels[${i}].name`)}</p>
              )}
            </div>
          ))}
          {issue("channels") && <p className="text-xs text-destructive">{issue("channels")}</p>}
          <div>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={content.channels.length >= LIMITS.channels}
              onClick={() => update((c) => ({ ...c, channels: [...c.channels, newChannel()] }))}
            >
              <Plus aria-hidden />
              Add channel
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
