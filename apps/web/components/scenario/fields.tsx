"use client";

import { useId, useState } from "react";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { formatClock, parseClock } from "@/lib/scenario/time";

interface ShellProps {
  label: string;
  hint?: string;
  error?: string;
  className?: string;
}

function Shell({
  id,
  label,
  hint,
  error,
  className,
  children,
}: ShellProps & { id: string; children: React.ReactNode }) {
  return (
    <div className={cn("grid content-start gap-1.5", className)}>
      <Label htmlFor={id}>{label}</Label>
      {children}
      {error ? (
        <p id={`${id}-msg`} className="text-xs text-destructive">
          {error}
        </p>
      ) : hint ? (
        <p id={`${id}-msg`} className="text-xs text-muted-foreground">
          {hint}
        </p>
      ) : null}
    </div>
  );
}

function aria(id: string, { hint, error }: ShellProps) {
  return {
    id,
    "aria-invalid": error ? (true as const) : undefined,
    "aria-describedby": error || hint ? `${id}-msg` : undefined,
  };
}

export function TextField({
  value,
  onChange,
  disabled,
  placeholder,
  maxLength,
  ...shell
}: ShellProps & {
  value: string;
  onChange: (v: string) => void;
  disabled?: boolean;
  placeholder?: string;
  maxLength?: number;
}) {
  const id = useId();
  return (
    <Shell id={id} {...shell}>
      <Input
        {...aria(id, shell)}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
        placeholder={placeholder}
        maxLength={maxLength}
      />
    </Shell>
  );
}

export function TextAreaField({
  value,
  onChange,
  disabled,
  placeholder,
  maxLength,
  rows = 3,
  ...shell
}: ShellProps & {
  value: string;
  onChange: (v: string) => void;
  disabled?: boolean;
  placeholder?: string;
  maxLength?: number;
  rows?: number;
}) {
  const id = useId();
  return (
    <Shell id={id} {...shell}>
      <Textarea
        {...aria(id, shell)}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        disabled={disabled}
        placeholder={placeholder}
        maxLength={maxLength}
        rows={rows}
      />
    </Shell>
  );
}

export const selectClass =
  "h-8 w-full min-w-0 rounded-lg border border-input bg-transparent px-2 text-sm outline-none transition-colors focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20";

export function SelectField<T extends string>({
  value,
  onChange,
  options,
  disabled,
  ...shell
}: ShellProps & {
  value: T;
  onChange: (v: T) => void;
  options: { value: T; label: string }[];
  disabled?: boolean;
}) {
  const id = useId();
  return (
    <Shell id={id} {...shell}>
      <select
        {...aria(id, shell)}
        className={selectClass}
        value={value}
        onChange={(e) => onChange(e.target.value as T)}
        disabled={disabled}
      >
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </Shell>
  );
}

/**
 * Text input bound to a number. While focused it shows what the user typed;
 * each keystroke that parses is committed, and on blur it snaps back to the
 * canonical form of the stored value.
 */
function ParsedInput({
  value,
  onChange,
  parse,
  format,
  disabled,
  inputMode,
  ...rest
}: {
  value: number;
  onChange: (v: number) => void;
  parse: (text: string) => number | null;
  format: (v: number) => string;
  disabled?: boolean;
  inputMode: "numeric" | "decimal";
} & ReturnType<typeof aria>) {
  const [draft, setDraft] = useState<string | null>(null);
  const invalid = draft !== null && parse(draft) === null;
  return (
    <Input
      {...rest}
      aria-invalid={invalid || rest["aria-invalid"] ? true : undefined}
      className="font-mono tabular-nums"
      inputMode={inputMode}
      autoComplete="off"
      value={draft ?? format(value)}
      disabled={disabled}
      onFocus={(e) => e.target.select()}
      onChange={(e) => {
        setDraft(e.target.value);
        const parsed = parse(e.target.value);
        if (parsed !== null) onChange(parsed);
      }}
      onBlur={() => setDraft(null)}
    />
  );
}

/** A time or duration, typed as mm:ss (or a bare number of minutes). */
export function ClockField({
  value,
  onChange,
  max = 86400,
  disabled,
  ...shell
}: ShellProps & { value: number; onChange: (sec: number) => void; max?: number; disabled?: boolean }) {
  const id = useId();
  return (
    <Shell id={id} {...shell}>
      <ParsedInput
        {...aria(id, shell)}
        value={value}
        onChange={onChange}
        format={formatClock}
        parse={(t) => {
          const sec = parseClock(t);
          return sec === null || sec > max ? null : sec;
        }}
        disabled={disabled}
        inputMode="numeric"
      />
    </Shell>
  );
}

export function NumberField({
  value,
  onChange,
  min,
  max,
  disabled,
  ...shell
}: ShellProps & { value: number; onChange: (v: number) => void; min: number; max: number; disabled?: boolean }) {
  const id = useId();
  return (
    <Shell id={id} {...shell}>
      <ParsedInput
        {...aria(id, shell)}
        value={value}
        onChange={onChange}
        format={String}
        parse={(t) => {
          if (!/^\d+$/.test(t.trim())) return null;
          const n = Number(t);
          return n < min || n > max ? null : n;
        }}
        disabled={disabled}
        inputMode="numeric"
      />
    </Shell>
  );
}
