import { useState, useRef, useEffect, type ReactNode } from "react";
import {
  Activity,
  GitCompareArrows,
  TrendingUp,
  Radio,
  Settings,
  Play,
  LogOut,
  ChevronDown,
} from "lucide-react";
import { useAuth } from "../contexts/AuthContext";

const navItems = [
  { href: "#/", label: "Runs" },
  { href: "#/compare", label: "Compare", icon: GitCompareArrows },
  { href: "#/trends", label: "Trends", icon: TrendingUp },
  { href: "#/live", label: "Live", icon: Radio },
];

function UserMenu() {
  const { user, logout } = useAuth();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handleClickOutside = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  if (!user) return null;

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen(!open)}
        className="flex items-center gap-2 rounded px-2 py-1 text-sm text-zinc-300 transition-colors hover:bg-zinc-800 hover:text-zinc-100"
      >
        <img
          src={user.avatarUrl}
          alt={user.login}
          className="h-6 w-6 rounded-full"
        />
        <span className="hidden sm:inline">{user.name || user.login}</span>
        <ChevronDown className="h-3.5 w-3.5" />
      </button>
      {open && (
        <div className="absolute right-0 top-full z-50 mt-1 w-48 rounded-lg border border-zinc-700 bg-zinc-800 py-1 shadow-lg">
          <div className="border-b border-zinc-700 px-3 py-2">
            <p className="text-sm font-medium text-zinc-100">
              {user.name || user.login}
            </p>
            <p className="text-xs text-zinc-400">@{user.login}</p>
          </div>
          <a
            href="#/settings"
            onClick={() => setOpen(false)}
            className="flex items-center gap-2 px-3 py-2 text-sm text-zinc-300 transition-colors hover:bg-zinc-700"
          >
            <Settings className="h-3.5 w-3.5" />
            Settings
          </a>
          <button
            onClick={() => {
              setOpen(false);
              void logout();
            }}
            className="flex w-full items-center gap-2 px-3 py-2 text-sm text-zinc-300 transition-colors hover:bg-zinc-700"
          >
            <LogOut className="h-3.5 w-3.5" />
            Sign out
          </button>
        </div>
      )}
    </div>
  );
}

export default function Layout({ children }: { children: ReactNode }) {
  const hash = typeof window !== "undefined" ? window.location.hash : "#/";

  return (
    <div className="min-h-screen bg-zinc-900">
      <header className="border-b border-zinc-800 px-6 py-4">
        <div className="flex items-center gap-6">
          <a href="#/" className="flex items-center gap-2 text-zinc-100">
            <Activity className="h-5 w-5 text-blue-500" />
            <span className="text-lg font-semibold tracking-tight">waza</span>
            <span className="text-sm text-zinc-500">eval dashboard</span>
          </a>
          <nav className="flex items-center gap-1">
            {navItems.map((item) => {
              const active =
                item.href === "#/"
                  ? hash === "#/" || hash === "" || hash === "#"
                  : hash === item.href;
              return (
                <a
                  key={item.href}
                  href={item.href}
                  className={`flex items-center gap-1.5 rounded px-3 py-1.5 text-sm transition-colors ${
                    active
                      ? "bg-zinc-800 text-zinc-100"
                      : "text-zinc-400 hover:text-zinc-200"
                  }`}
                >
                  {item.icon && <item.icon className="h-3.5 w-3.5" />}
                  {item.label}
                </a>
              );
            })}
          </nav>
          <div className="ml-auto flex items-center gap-3">
            <a
              href="#/runs/new"
              className="inline-flex items-center gap-1.5 rounded bg-blue-600 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-blue-500"
            >
              <Play className="h-3.5 w-3.5" />
              New Run
            </a>
            <a
              href="#/settings"
              className={`flex items-center rounded p-1.5 transition-colors ${
                hash === "#/settings"
                  ? "bg-zinc-800 text-zinc-100"
                  : "text-zinc-400 hover:text-zinc-200"
              }`}
              title="Settings"
            >
              <Settings className="h-4 w-4" />
            </a>
            <UserMenu />
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-7xl px-6 py-8">{children}</main>
    </div>
  );
}
