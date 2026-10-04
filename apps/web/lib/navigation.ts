import type { Role } from "@/lib/auth/roles";

export type NavIcon =
  | "dashboard"
  | "scenarios"
  | "sessions"
  | "aar"
  | "users"
  | "config"
  | "audit"
  | "history";

export interface NavItem {
  label: string;
  href: string;
  icon: NavIcon;
  /** Planned but not built yet: rendered disabled so there are no dead links. */
  planned?: boolean;
}

const NAV: Record<Role, NavItem[]> = {
  INSTRUCTOR: [
    { label: "Dashboard", href: "/instructor", icon: "dashboard" },
    { label: "Scenarios", href: "/instructor/scenarios", icon: "scenarios" },
    { label: "Sessions", href: "/instructor/sessions", icon: "sessions", planned: true },
    { label: "After Action Reviews", href: "/instructor/aar", icon: "aar", planned: true },
  ],
  TRAINEE: [
    { label: "Dashboard", href: "/trainee", icon: "dashboard" },
    { label: "My exercises", href: "/trainee/sessions", icon: "sessions", planned: true },
    { label: "Decision history", href: "/trainee/history", icon: "history", planned: true },
  ],
  ADMIN: [
    { label: "Dashboard", href: "/admin", icon: "dashboard" },
    { label: "Users", href: "/admin/users", icon: "users", planned: true },
    { label: "Configuration", href: "/admin/config", icon: "config", planned: true },
    { label: "Audit log", href: "/admin/audit", icon: "audit", planned: true },
  ],
};

export function navForRole(role: Role): NavItem[] {
  return NAV[role];
}
