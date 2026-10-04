"use client";

import { LoaderCircle } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";
import type { z } from "zod";

import { FormField } from "@/components/shared/form-field";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { apiFetch } from "@/lib/api/client";
import { ApiError } from "@/lib/api/errors";
import type { AuthResponse } from "@/lib/api/types";
import { resolvePostLoginPath } from "@/lib/auth/roles";
import { fieldErrors, type FieldErrors } from "@/lib/validation/auth";

export interface AuthField {
  name: string;
  label: string;
  type: "text" | "email" | "password";
  autoComplete: string;
  hint?: string;
}

/**
 * Shared sign-in / sign-up form: validates locally, posts to the API, shows
 * field and form errors, then routes the user to their role's area.
 */
export function AuthForm({
  endpoint,
  schema,
  fields,
  submitLabel,
  pendingLabel,
  next,
}: {
  endpoint: "/auth/login" | "/auth/register";
  schema: z.ZodType<Record<string, string>>;
  fields: AuthField[];
  submitLabel: string;
  pendingLabel: string;
  next?: string;
}) {
  const router = useRouter();
  const [errors, setErrors] = useState<FieldErrors>({});
  const [formError, setFormError] = useState<string | null>(null);
  const [pending, setPending] = useState(false);

  async function onSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (pending) return;

    const raw = Object.fromEntries(new FormData(event.currentTarget));
    const parsed = schema.safeParse(raw);
    if (!parsed.success) {
      setErrors(fieldErrors(parsed.error));
      setFormError(null);
      return;
    }

    setErrors({});
    setFormError(null);
    setPending(true);
    try {
      const { user } = await apiFetch<AuthResponse>(endpoint, { body: parsed.data });
      router.replace(resolvePostLoginPath(user.role, next));
      router.refresh();
      // Stay pending: the page is navigating away.
    } catch (err) {
      setPending(false);
      if (err instanceof ApiError) {
        setErrors(err.details);
        setFormError(Object.keys(err.details).length > 0 ? null : err.message);
      } else {
        setFormError("Something went wrong. Please try again.");
      }
    }
  }

  return (
    <form onSubmit={onSubmit} noValidate className="grid gap-4">
      {formError && (
        <Alert variant="destructive">
          <AlertDescription>{formError}</AlertDescription>
        </Alert>
      )}
      {fields.map((f) => (
        <FormField
          key={f.name}
          id={f.name}
          name={f.name}
          label={f.label}
          type={f.type}
          autoComplete={f.autoComplete}
          hint={f.hint}
          error={errors[f.name]}
          disabled={pending}
          required
        />
      ))}
      <Button type="submit" size="lg" disabled={pending} aria-busy={pending}>
        {pending && <LoaderCircle className="animate-spin" aria-hidden />}
        {pending ? pendingLabel : submitLabel}
      </Button>
    </form>
  );
}
