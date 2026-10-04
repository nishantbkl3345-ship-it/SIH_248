import { describe, expect, it } from "vitest";

import {
  homeForRole,
  isProtectedPath,
  resolvePostLoginPath,
  roleForPath,
} from "@/lib/auth/roles";

describe("roleForPath", () => {
  it("maps each area to its role", () => {
    expect(roleForPath("/admin")).toBe("ADMIN");
    expect(roleForPath("/instructor/scenarios/42")).toBe("INSTRUCTOR");
    expect(roleForPath("/trainee/sessions")).toBe("TRAINEE");
  });

  it("does not match look-alike prefixes or public pages", () => {
    expect(roleForPath("/administrator")).toBeNull();
    expect(roleForPath("/login")).toBeNull();
    expect(roleForPath("/")).toBeNull();
    expect(isProtectedPath("/trainees")).toBe(false);
    expect(isProtectedPath("/trainee")).toBe(true);
  });
});

describe("resolvePostLoginPath", () => {
  it("defaults to the role home", () => {
    expect(resolvePostLoginPath("TRAINEE")).toBe(homeForRole("TRAINEE"));
    expect(resolvePostLoginPath("ADMIN", "")).toBe("/admin");
  });

  it("honours a next path inside the user's own area", () => {
    expect(resolvePostLoginPath("INSTRUCTOR", "/instructor/sessions?tab=live")).toBe(
      "/instructor/sessions?tab=live",
    );
  });

  it("ignores another role's area", () => {
    expect(resolvePostLoginPath("TRAINEE", "/admin/users")).toBe("/trainee");
  });

  it("rejects open redirects", () => {
    for (const next of [
      "https://evil.example/instructor",
      "//evil.example/instructor",
      "/\\evil.example",
      "instructor",
      "javascript:alert(1)",
    ]) {
      expect(resolvePostLoginPath("INSTRUCTOR", next)).toBe("/instructor");
    }
  });
});
