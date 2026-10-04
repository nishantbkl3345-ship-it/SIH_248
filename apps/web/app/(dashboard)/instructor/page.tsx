import type { Metadata } from "next";
import Link from "next/link";

import { PageHeader } from "@/components/shared/page-header";
import { PlannedFeatures } from "@/components/shared/planned-features";
import { StatCard } from "@/components/shared/stat-card";
import { buttonVariants } from "@/components/ui/button";
import { serverApi } from "@/lib/api/server";
import { requireRole } from "@/lib/auth/session";
import type { ScenarioSummary } from "@/lib/scenario/types";

export const metadata: Metadata = { title: "Instructor" };

export default async function InstructorDashboard() {
  const user = await requireRole("INSTRUCTOR");
  const { scenarios } = await serverApi<{ scenarios: ScenarioSummary[] }>("/scenarios");
  const published = scenarios.filter((s) => s.status === "PUBLISHED").length;
  return (
    <>
      <PageHeader
        title={`Welcome, ${user.displayName}`}
        description="Build scenarios, run exercises with degraded communications, and review how trainees decided."
      >
        <Link href="/instructor/scenarios" className={buttonVariants()}>
          Open scenario library
        </Link>
      </PageHeader>
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <StatCard
          label="Scenarios"
          value={scenarios.length}
          hint={`${published} published, ${scenarios.length - published} in draft.`}
        />
        <StatCard label="Live sessions" value="—" hint="Available once exercises can be run." />
        <StatCard label="After Action Reviews" value="—" hint="Available once reviews are generated." />
      </div>
      <PlannedFeatures
        title="Instructor workspace"
        description="Scenario authoring is available now. These areas arrive in later phases."
        features={[
          { title: "Exercise lobby and control", description: "Share a join code, assign teams, then start, pause, resume and end." },
          { title: "Live monitor", description: "Compare what was sent with what each trainee actually received." },
          { title: "After Action Review", description: "Timelines, information availability, decisions and metrics, exportable." },
        ]}
      />
    </>
  );
}
