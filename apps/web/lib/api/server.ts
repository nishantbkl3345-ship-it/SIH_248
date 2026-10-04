import "server-only";

import { cookies } from "next/headers";

import { toApiError } from "@/lib/api/errors";
import { SESSION_COOKIE } from "@/lib/auth/roles";
import { env } from "@/lib/env";

/**
 * Server-side API call made on behalf of the signed-in user. Never cached:
 * every response depends on who is asking.
 */
export async function serverApi<T>(path: string): Promise<T> {
  const token = (await cookies()).get(SESSION_COOKIE)?.value;
  const res = await fetch(`${env.API_URL}/api/v1${path}`, {
    headers: token ? { Authorization: `Bearer ${token}` } : undefined,
    cache: "no-store",
  });
  if (!res.ok) throw await toApiError(res);
  return (await res.json()) as T;
}
