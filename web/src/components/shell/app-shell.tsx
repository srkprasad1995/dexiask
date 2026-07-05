"use client";

import { Suspense, useState, type ReactNode } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { Brain, Database, Menu, MessageCircle, Plug, Users } from "lucide-react";

import { cn } from "@/lib/utils";
import { useIsAdmin } from "@/lib/auth/use-user";
import { Button } from "@/components/ui/button";
import { Sheet, SheetContent, SheetTitle } from "@/components/ui/sheet";
import { ThemeToggle } from "@/components/theme/theme-toggle";
import { DexiaskAvatar } from "@/components/brand/dexiask-avatar";
import { UserBadge } from "@/components/shell/user-badge";
import { ConversationHistory } from "@/components/shell/conversation-history";

const NAV = [
  { href: "/", label: "Chat", icon: MessageCircle, adminOnly: false },
  { href: "/indexer", label: "Indexer", icon: Database, adminOnly: false },
  { href: "/memory", label: "Memory", icon: Brain, adminOnly: false },
  { href: "/mcp", label: "MCP", icon: Plug, adminOnly: true },
  { href: "/admin", label: "Team", icon: Users, adminOnly: true },
] as const;

function Wordmark() {
  return (
    <Link href="/" className="flex items-center gap-2 px-1">
      <DexiaskAvatar size={26} round={false} />
      <span className="text-base font-semibold tracking-tight lowercase">
        dexiask
      </span>
    </Link>
  );
}

function NavLinks({ onNavigate }: { onNavigate?: () => void }) {
  const pathname = usePathname();
  const isAdmin = useIsAdmin();
  return (
    <nav className="flex flex-col gap-1">
      {NAV.filter((item) => !item.adminOnly || isAdmin).map(({ href, label, icon: Icon }) => {
        const active = href === "/" ? pathname === "/" : pathname.startsWith(href);
        return (
          <Link
            key={href}
            href={href}
            onClick={onNavigate}
            className={cn(
              "flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm font-medium transition-colors",
              active
                ? "bg-primary/10 text-primary"
                : "text-muted-foreground hover:bg-muted hover:text-foreground",
            )}
          >
            <Icon className="h-4 w-4 shrink-0" />
            {label}
          </Link>
        );
      })}
    </nav>
  );
}

function Sidebar({ onNavigate }: { onNavigate?: () => void }) {
  return (
    <div className="flex h-full min-h-0 flex-col gap-4 p-3">
      <div className="pt-1">
        <Wordmark />
      </div>
      <NavLinks onNavigate={onNavigate} />
      <div className="-mx-1 min-h-0 flex-1 border-t pt-2">
        <Suspense
          fallback={<p className="px-2 py-4 text-xs text-muted-foreground">Loading…</p>}
        >
          <ConversationHistory onNavigate={onNavigate} />
        </Suspense>
      </div>
    </div>
  );
}

/**
 * Minimal app shell: a left rail with the Dexiask wordmark + Chat/Indexer nav,
 * a top bar with the page title and a theme toggle, and the page content. On
 * mobile the rail collapses into a sheet.
 */
export function AppShell({
  title,
  children,
}: {
  title?: string;
  children: ReactNode;
}) {
  const [mobileOpen, setMobileOpen] = useState(false);

  return (
    <div className="flex h-dvh w-full overflow-hidden">
      <aside className="hidden w-56 shrink-0 border-r bg-sidebar md:block">
        <Sidebar />
      </aside>

      <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
        <SheetContent side="left" className="w-56 p-0">
          <SheetTitle className="sr-only">Navigation</SheetTitle>
          <Sidebar onNavigate={() => setMobileOpen(false)} />
        </SheetContent>
      </Sheet>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 shrink-0 items-center gap-2 border-b px-4">
          <Button
            variant="ghost"
            size="icon"
            className="md:hidden"
            aria-label="Open navigation"
            onClick={() => setMobileOpen(true)}
          >
            <Menu className="h-5 w-5" />
          </Button>
          {title && <h1 className="text-sm font-medium">{title}</h1>}
          <div className="ml-auto flex items-center gap-1">
            <ThemeToggle />
            <UserBadge />
          </div>
        </header>
        <main className="min-h-0 flex-1 overflow-hidden">{children}</main>
      </div>
    </div>
  );
}
