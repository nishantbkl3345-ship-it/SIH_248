import "server-only";

import { redirect } from "next/navigation";
import { cache } from "react";

import { ApiError } from "@/lib/api/errors";
import { serverApi } from "@/lib/api/server";
import type { User } from "@/lib/api/types";
import { homeForRole, type Role } from "@/lib/auth/roles";

/**
 * The signed-in user, verified by the API, or null. Deduplicated per request.
 * API outages are thrown (and reach the error boundary) rather than being
 * mistaken for "signed out".
 */
export const getCurrentUser = cache(async (): Promise<User | null> => {
  try {
    const { user } = await serverApi<{ user: User }>("/auth/me");
    return user;
  } catch (err) {
    if (err instanceof ApiError && err.status === 401) return null;
    throw err;
  }
});

export async function requireUser(): Promise<User> {
  const user = await getCurrentUser();
  if (!user) redirect("/login");
  return user;
}

/** Sends users without the role back to their own area. */
export async function requireRole(role: Role): Promise<User> {
  const user = await requireUser();
  if (user.role !== role) redirect(homeForRole(user.role));
  return user;
}
