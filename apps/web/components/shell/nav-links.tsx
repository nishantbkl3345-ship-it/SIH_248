"use client";

import {
  BookOpen,
  ClipboardList,
  FileText,
  History,
  LayoutDashboard,
  ScrollText,
  Settings,
  Users,
  type LucideIcon,
} from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";

import { cn } from "@/lib/utils";
import type { NavIcon, NavItem } from "@/lib/navigation";

const ICONS: Record<NavIcon, LucideIcon> = {
  dashboard: LayoutDashboard,
  scenarios: BookOpen,
  sessions: ClipboardList,
  aar: FileText,
  users: Users,
  config: Settings,
  audit: ScrollText,
  history: History,
};

const itemClass =
  "flex items-center gap-2.5 rounded-md px-2.5 py-2 text-sm font-medium transition-colors";

export function NavLinks({
  items,
  onNavigate,
}: {
  items: NavItem[];
  onNavigate?: () => void;
}) {
  const pathname = usePathname();
  // The longest matching href wins, so "/admin" is not active on "/admin/users".
  const active = items
    .filter((i) => !i.planned && (pathname === i.href || pathname.startsWith(`${i.href}/`)))
    .sort((a, b) => b.href.length - a.href.length)[0]?.href;

  return (
    <nav aria-label="Main" className="grid gap-1">
      {items.map((item) => {
        const Icon = ICONS[item.icon];
        if (item.planned) {
          return (
            <span
              key={item.href}
              aria-disabled="true"
              className={cn(itemClass, "cursor-not-allowed text-muted-foreground/70")}
            >
              <Icon className="size-4 shrink-0" aria-hidden />
              <span className="truncate">{item.label}</span>
              <span className="ml-auto text-[0.65rem] font-normal uppercase tracking-wide">
                Soon
              </span>
            </span>
          );
        }
        const isActive = item.href === active;
        return (
          <Link
            key={item.href}
            href={item.href}
            onClick={onNavigate}
            aria-current={isActive ? "page" : undefined}
            className={cn(
              itemClass,
              isActive
                ? "bg-accent text-accent-foreground"
                : "text-muted-foreground hover:bg-accent/60 hover:text-foreground",
            )}
          >
            <Icon className="size-4 shrink-0" aria-hidden />
            <span className="truncate">{item.label}</span>
          </Link>
        );
      })}
    </nav>
  );
}
