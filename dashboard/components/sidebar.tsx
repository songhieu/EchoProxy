"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { signOut } from "next-auth/react";
import {
  Activity,
  BarChart3,
  ChevronsUpDown,
  Code2,
  Folder,
  KeyRound,
  LogOut,
  Plus,
  ScrollText,
  Settings,
  Zap,
} from "lucide-react";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Separator } from "@/components/ui/separator";
import { cn } from "@/lib/utils";
import type { Project } from "@/lib/api/types";

export function Sidebar({
  email,
  projects,
  currentProjectId,
}: {
  email: string;
  projects: Project[];
  currentProjectId?: string;
}) {
  const pathname = usePathname();
  const current = projects.find((p) => String(p.id) === currentProjectId);

  return (
    <aside className="flex h-screen w-60 flex-col border-r bg-card">
      <div className="flex h-14 items-center gap-2 border-b px-4 font-semibold">
        <Zap className="h-5 w-5 text-primary" />
        EchoProxy
      </div>

      <div className="px-3 pt-3">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" className="w-full justify-between font-normal">
              <span className="flex items-center gap-2 truncate">
                <Folder className="h-4 w-4 shrink-0" />
                <span className="truncate">{current ? current.name : "Select project"}</span>
              </span>
              <ChevronsUpDown className="h-4 w-4 opacity-50" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent className="w-56" align="start">
            <DropdownMenuLabel>Projects</DropdownMenuLabel>
            <DropdownMenuSeparator />
            {projects.length === 0 && (
              <div className="px-2 py-1.5 text-sm text-muted-foreground">No projects yet</div>
            )}
            {projects.map((p) => (
              <DropdownMenuItem key={p.id} asChild>
                <Link href={`/projects/${p.id}/keys`}>
                  <Folder /> {p.name}
                </Link>
              </DropdownMenuItem>
            ))}
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link href="/projects">
                <Plus /> New project
              </Link>
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <nav className="mt-4 flex flex-col gap-0.5 px-2 text-sm">
        {currentProjectId ? (
          <>
            <NavItem href={`/projects/${currentProjectId}/integrate`} active={pathname?.includes("/integrate")}>
              <Code2 /> Integrate
            </NavItem>
            <NavItem href={`/projects/${currentProjectId}/keys`} active={pathname?.includes("/keys")}>
              <KeyRound /> API keys
            </NavItem>
            <NavItem href={`/projects/${currentProjectId}/logs`} active={pathname?.endsWith("/logs")}>
              <ScrollText /> Logs
            </NavItem>
            <NavItem href={`/projects/${currentProjectId}/analytics`} active={pathname?.endsWith("/analytics")}>
              <BarChart3 /> Analytics
            </NavItem>
            <NavItem href={`/projects/${currentProjectId}/live`} active={pathname?.endsWith("/live")}>
              <Activity /> Live tail
            </NavItem>
            <NavItem href={`/projects/${currentProjectId}/settings`} active={pathname?.endsWith("/settings")}>
              <Settings /> Settings
            </NavItem>
          </>
        ) : (
          <NavItem href="/projects" active={pathname === "/projects"}>
            <Folder /> All projects
          </NavItem>
        )}
      </nav>

      <div className="mt-auto border-t p-3">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button className="flex w-full items-center gap-3 rounded-md px-2 py-1.5 hover:bg-accent">
              <Avatar>
                <AvatarFallback>{email.slice(0, 2).toUpperCase()}</AvatarFallback>
              </Avatar>
              <div className="min-w-0 flex-1 text-left">
                <div className="truncate text-sm font-medium">{email}</div>
                <div className="text-xs text-muted-foreground">Account</div>
              </div>
              <ChevronsUpDown className="h-4 w-4 opacity-50" />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent className="w-52" align="end">
            <DropdownMenuLabel className="truncate">{email}</DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem asChild>
              <Link href="/projects">
                <Settings /> Projects
              </Link>
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem onSelect={() => signOut({ callbackUrl: "/login" })}>
              <LogOut /> Sign out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </aside>
  );
}

function NavItem({
  href,
  active,
  children,
}: {
  href: string;
  active?: boolean;
  children: React.ReactNode;
}) {
  return (
    <Link
      href={href}
      className={cn(
        "flex items-center gap-2 rounded-md px-3 py-2 text-sm transition-colors hover:bg-accent",
        active && "bg-accent font-medium text-accent-foreground",
      )}
    >
      {children}
    </Link>
  );
}

export function ProjectSidebarSkeleton() {
  return (
    <aside className="flex h-screen w-60 flex-col border-r bg-card">
      <div className="flex h-14 items-center gap-2 border-b px-4 font-semibold">
        <Zap className="h-5 w-5 text-primary" />
        EchoProxy
      </div>
      <Separator />
    </aside>
  );
}
