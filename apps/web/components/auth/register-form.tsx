"use client";

import { AuthForm } from "@/components/auth/auth-form";
import { registerSchema } from "@/lib/validation/auth";

export function RegisterForm() {
  return (
    <AuthForm
      endpoint="/auth/register"
      schema={registerSchema}
      submitLabel="Create account"
      pendingLabel="Creating account…"
      fields={[
        { name: "displayName", label: "Name", type: "text", autoComplete: "name" },
        { name: "email", label: "Email", type: "email", autoComplete: "email" },
        {
          name: "password",
          label: "Password",
          type: "password",
          autoComplete: "new-password",
          hint: "At least 8 characters.",
        },
      ]}
    />
  );
}
