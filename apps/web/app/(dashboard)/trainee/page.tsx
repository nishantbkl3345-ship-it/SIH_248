import type { Metadata } from "next";

import { PageHeader } from "@/components/shared/page-header";
import { PlannedFeatures } from "@/components/shared/planned-features";
import { StatCard } from "@/components/shared/stat-card";
import { requireRole } from "@/lib/auth/session";

export const metadata: Metadata = { title: "Trainee" };

export default async function TraineeDashboard() {
  const user = await requireRole("TRAINEE");
  return (
    <>
      <PageHeader
        title={`Welcome, ${user.displayName}`}
        description="Join an exercise, work with the information you are given, and record your decisions and reasoning."
      />
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <StatCard label="Exercises joined" value="—" hint="Available once exercises can be run." />
        <StatCard label="Decisions recorded" value="—" hint="Available once exercises can be run." />
        <StatCard label="Reviews released to you" value="—" hint="Available once reviews are generated." />
      </div>
      <PlannedFeatures
        title="Trainee workspace"
        description="This dashboard is a placeholder. These areas arrive in later phases."
        features={[
          { title: "Join by code", description: "Enter the code your instructor shares to enter the exercise lobby." },
          { title: "Briefing", description: "Read the fictional situation, your seat and the channels available to you." },
          { title: "Incoming reports", description: "See only the information that actually reaches you." },
          { title: "Team communications", description: "Message your team over channels that may be delayed or cut." },
          { title: "Decision panel", description: "Choose an option, explain your rationale and state your confidence." },
        ]}
      />
    </>
  );
}
