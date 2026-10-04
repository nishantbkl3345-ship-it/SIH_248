"use client";

import { AuthForm } from "@/components/auth/auth-form";
import { loginSchema } from "@/lib/validation/auth";

export function LoginForm({ next }: { next?: string }) {
  return (
    <AuthForm
      endpoint="/auth/login"
      schema={loginSchema}
      next={next}
      submitLabel="Sign in"
      pendingLabel="Signing in…"
      fields={[
        { name: "email", label: "Email", type: "email", autoComplete: "email" },
        { name: "password", label: "Password", type: "password", autoComplete: "current-password" },
      ]}
    />
  );
}
