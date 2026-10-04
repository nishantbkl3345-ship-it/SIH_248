import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { ScenarioBuilder } from "@/components/scenario/scenario-builder";
import { ApiError } from "@/lib/api/errors";
import { serverApi } from "@/lib/api/server";
import { requireRole } from "@/lib/auth/session";
import type { Scenario } from "@/lib/scenario/types";

export const metadata: Metadata = { title: "Scenario builder" };

export default async function ScenarioPage({ params }: { params: Promise<{ id: string }> }) {
  await requireRole("INSTRUCTOR");
  const { id } = await params;

  let scenario: Scenario;
  try {
    ({ scenario } = await serverApi<{ scenario: Scenario }>(`/scenarios/${encodeURIComponent(id)}`));
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) notFound();
    throw err;
  }

  // Keyed by id so opening another scenario starts with fresh editor state.
  return <ScenarioBuilder key={scenario.id} initial={scenario} />;
}
