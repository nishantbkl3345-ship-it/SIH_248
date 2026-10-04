import type { Metadata } from "next";

import { NewScenarioButton, ScenarioLibrary } from "@/components/scenario/scenario-library";
import { PageHeader } from "@/components/shared/page-header";
import { serverApi } from "@/lib/api/server";
import { requireRole } from "@/lib/auth/session";
import type { ScenarioSummary } from "@/lib/scenario/types";

export const metadata: Metadata = { title: "Scenarios" };

export default async function ScenariosPage() {
  await requireRole("INSTRUCTOR");
  const { scenarios } = await serverApi<{ scenarios: ScenarioSummary[] }>("/scenarios");

  return (
    <>
      <PageHeader
        title="Scenario library"
        description="Fictional exercises you have built. Drafts can be edited; published scenarios are locked and ready to run."
      >
        {scenarios.length > 0 && <NewScenarioButton />}
      </PageHeader>
      <ScenarioLibrary scenarios={scenarios} />
    </>
  );
}
