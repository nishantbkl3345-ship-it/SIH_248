import { describe, expect, it } from "vitest";

import { fieldErrors, loginSchema, registerSchema } from "@/lib/validation/auth";

describe("auth schemas", () => {
  it("accepts valid input and trims the name", () => {
    const r = registerSchema.parse({
      displayName: "  Mira  ",
      email: "mira@example.test",
      password: "password-123",
    });
    expect(r.displayName).toBe("Mira");
    expect(loginSchema.safeParse({ email: "mira@example.test", password: "x" }).success).toBe(true);
  });

  it("reports one message per invalid field", () => {
    const r = registerSchema.safeParse({ displayName: " ", email: "nope", password: "short" });
    expect(r.success).toBe(false);
    if (!r.success) {
      expect(Object.keys(fieldErrors(r.error)).sort()).toEqual(["displayName", "email", "password"]);
    }
  });

  it("enforces the 72-character password limit shared with the API", () => {
    const r = registerSchema.safeParse({
      displayName: "A",
      email: "a@example.test",
      password: "p".repeat(73),
    });
    expect(r.success).toBe(false);
  });
});
