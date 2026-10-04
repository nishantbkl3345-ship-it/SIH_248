import type { Metadata } from "next";

import { PageHeader } from "@/components/shared/page-header";
import { PlannedFeatures } from "@/components/shared/planned-features";
import { StatCard } from "@/components/shared/stat-card";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { serverApi } from "@/lib/api/server";
import type { UserListResponse } from "@/lib/api/types";
import { ROLE_LABEL } from "@/lib/auth/roles";
import { requireRole } from "@/lib/auth/session";

export const metadata: Metadata = { title: "Administration" };

const PAGE_SIZE = 50;

const dateTime = new Intl.DateTimeFormat("en-GB", {
  dateStyle: "medium",
  timeStyle: "short",
  timeZone: "UTC",
});

export default async function AdminDashboard() {
  await requireRole("ADMIN");
  const { users, total } = await serverApi<UserListResponse>(`/admin/users?limit=${PAGE_SIZE}`);

  return (
    <>
      <PageHeader
        title="Administration"
        description="Manage who can use the system. Configuration and the audit log arrive in later phases."
      />
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <StatCard label="Users" value={total} />
        <StatCard label="Configuration" value="—" hint="Available once system settings are built." />
        <StatCard label="Audit events" value="—" hint="Available once the audit log is built." />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Users</CardTitle>
          <CardDescription>
            {total > users.length
              ? `Showing the first ${users.length} of ${total} accounts.`
              : "Every account, oldest first. Read-only for now."}
          </CardDescription>
        </CardHeader>
        <CardContent>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Email</TableHead>
                <TableHead>Role</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Last sign-in (UTC)</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {users.map((u) => (
                <TableRow key={u.id}>
                  <TableCell className="font-medium">{u.displayName}</TableCell>
                  <TableCell>{u.email}</TableCell>
                  <TableCell>{ROLE_LABEL[u.role]}</TableCell>
                  <TableCell>
                    <Badge variant={u.isActive ? "secondary" : "outline"}>
                      {u.isActive ? "Active" : "Deactivated"}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {u.lastLoginAt ? dateTime.format(new Date(u.lastLoginAt)) : "Never"}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <PlannedFeatures
        title="Administration tools"
        description="The API for creating and editing users exists; these screens do not yet."
        features={[
          { title: "User management", description: "Create accounts, change roles and deactivate users from this page." },
          { title: "System configuration", description: "Registration policy and exercise defaults." },
          { title: "Audit log", description: "Who signed in, changed roles, controlled sessions or exported reviews." },
        ]}
      />
    </>
  );
}
