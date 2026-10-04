import { redirect } from "next/navigation";

import { homeForRole } from "@/lib/auth/roles";
import { getCurrentUser } from "@/lib/auth/session";

export default async function RootPage() {
  const user = await getCurrentUser();
  redirect(user ? homeForRole(user.role) : "/login");
}
