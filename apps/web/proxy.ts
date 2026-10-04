import { NextResponse, type NextRequest } from "next/server";

import { isProtectedPath, SESSION_COOKIE } from "@/lib/auth/roles";

// Optimistic check only: no session cookie means no point rendering a
// protected page. The cookie is verified by the API in the layouts
// (lib/auth/session.ts), which is where access is actually decided.
export function proxy(request: NextRequest) {
  const { pathname, search } = request.nextUrl;
  if (isProtectedPath(pathname) && !request.cookies.has(SESSION_COOKIE)) {
    const login = new URL("/login", request.url);
    login.searchParams.set("next", pathname + search);
    return NextResponse.redirect(login);
  }
  return NextResponse.next();
}

export const config = {
  matcher: ["/instructor/:path*", "/trainee/:path*", "/admin/:path*"],
};
