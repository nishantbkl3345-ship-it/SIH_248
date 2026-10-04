export const ROLES = ["ADMIN", "INSTRUCTOR", "TRAINEE"] as const;
export type Role = (typeof ROLES)[number];

// Must match SessionCookie in the API (internal/transport/httpapi).
export const SESSION_COOKIE = "fogline_token";

const ROLE_HOME: Record<Role, string> = {
  ADMIN: "/admin",
  INSTRUCTOR: "/instructor",
  TRAINEE: "/trainee",
};

export const ROLE_LABEL: Record<Role, string> = {
  ADMIN: "Administrator",
  INSTRUCTOR: "Instructor",
  TRAINEE: "Trainee",
};

export function homeForRole(role: Role): string {
  return ROLE_HOME[role];
}

function isUnder(pathname: string, base: string): boolean {
  return pathname === base || pathname.startsWith(`${base}/`);
}

/** The role that owns a path, or null if the path is not role-restricted. */
export function roleForPath(pathname: string): Role | null {
  for (const role of ROLES) {
    if (isUnder(pathname, ROLE_HOME[role])) return role;
  }
  return null;
}

export function isProtectedPath(pathname: string): boolean {
  return roleForPath(pathname) !== null;
}

/**
 * Where to send a user after sign-in. `next` comes from the query string, so
 * it is only honoured when it is a local path inside the user's own area.
 */
export function resolvePostLoginPath(role: Role, next?: string | null): string {
  const home = homeForRole(role);
  if (!next || !next.startsWith("/") || next.startsWith("//") || next.includes("\\")) {
    return home;
  }
  const pathname = next.split(/[?#]/)[0];
  return roleForPath(pathname) === role ? next : home;
}
