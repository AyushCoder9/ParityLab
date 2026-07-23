"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useMemo, useRef, useState } from "react";
import { BrandMark, Icon, StatusPill } from "@paritylab/ui";
import { checkEngine, getEnvironments, getFindings, getNotifications, logout } from "@/lib/api";
import { useAuth } from "./auth-context";

const nav = [
  { label: "Overview", href: "/dashboard", icon: "dashboard" as const },
  { label: "Scenarios", href: "/scenarios", icon: "simulation" as const },
  { label: "Runs", href: "/runs", icon: "event" as const },
  { label: "Findings", href: "/findings", icon: "finding" as const },
  { label: "Reports", href: "/reports", icon: "report" as const },
  { label: "Connections", href: "/connections", icon: "spark" as const },
  { label: "Environments", href: "/environments", icon: "team" as const },
  { label: "Settings", href: "/settings", icon: "settings" as const },
];

const commands = [
  { label: "Run duplicate webhook scenario", href: "/scenarios?query=duplicate", icon: "simulation" as const },
  { label: "Open unresolved findings", href: "/findings?status=open", icon: "finding" as const },
  { label: "Configure Stripe Sandbox", href: "/connections", icon: "spark" as const },
  { label: "View verification reports", href: "/reports", icon: "report" as const },
];

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { session } = useAuth();
  const [commandOpen, setCommandOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [accountOpen, setAccountOpen] = useState(false);
  const [moreOpen, setMoreOpen] = useState(false);
  const [engineOnline, setEngineOnline] = useState(false);
  const [selectedEnvironment, setSelectedEnvironment] = useState("Environment");
  const [unreadNotifications, setUnreadNotifications] = useState(0);
  const [openFindings, setOpenFindings] = useState(0);
  const [signOutError, setSignOutError] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const controller = new AbortController();
    checkEngine(controller.signal).then(setEngineOnline).catch(() => setEngineOnline(false));
    return () => controller.abort();
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    const loadResources = () => {
      void Promise.allSettled([
        getEnvironments(controller.signal).then((response) => setSelectedEnvironment(response.data.find((item) => item.is_default)?.name ?? "Environment")),
        getNotifications(controller.signal).then((response) => setUnreadNotifications(response.data.filter((item) => !item.read_at).length)),
        getFindings("open", controller.signal).then((response) => setOpenFindings(response.data.length)),
      ]);
    };
    loadResources();
    window.addEventListener("paritylab:resources-changed", loadResources);
    return () => {
      controller.abort();
      window.removeEventListener("paritylab:resources-changed", loadResources);
    };
  }, []);

  useEffect(() => {
    function onKeyDown(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setCommandOpen((open) => !open);
      }
      if (event.key === "Escape") {
        setCommandOpen(false);
        setAccountOpen(false);
        setMoreOpen(false);
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  useEffect(() => {
    if (commandOpen) window.setTimeout(() => inputRef.current?.focus(), 0);
  }, [commandOpen]);

  const results = useMemo(() => commands.filter((command) => command.label.toLowerCase().includes(query.toLowerCase())), [query]);
  const initials = session?.user.email.slice(0, 2).toUpperCase() ?? "PL";

  function runCommand(href: string) {
    setCommandOpen(false);
    setQuery("");
    router.push(href);
  }

  async function signOut() {
    setSignOutError("");
    try {
      await logout();
      router.replace("/login");
      router.refresh();
    } catch {
      setSignOutError("Sign out could not be completed. Your session remains active; retry when the API is online.");
      setAccountOpen(false);
      setMoreOpen(false);
    }
  }

  return (
    <main className="app-shell">
      <a className="skip-link" href="#product-content">Skip to product content</a>
      <aside className="app-sidebar">
        <Link href="/" className="brand-link"><BrandMark /></Link>
        <Link href="/environments" className="workspace-switcher" aria-label="Open environments">
          <span>{session?.organization.name.slice(0, 2).toUpperCase() ?? "PL"}</span><p><strong>{session?.organization.name ?? "Workspace"}</strong><small>{session?.project.name ?? "Sandbox project"}</small></p><Icon name="chevron" />
        </Link>
        <nav aria-label="Product navigation">
          {nav.map((item, index) => {
            const active = pathname === item.href || (item.href !== "/dashboard" && pathname.startsWith(`${item.href}/`));
            return (
              <Link key={item.href} href={item.href} className={active ? "is-active" : ""} aria-current={active ? "page" : undefined}>
                <Icon name={item.icon} /><span>{item.label}</span>{item.label === "Findings" && openFindings ? <em>{openFindings}</em> : null}{index === 4 ? <i /> : null}
              </Link>
            );
          })}
          <button className="mobile-more-trigger" aria-label="More product navigation" aria-expanded={moreOpen} onClick={() => setMoreOpen((open) => !open)}><Icon name="command"/><span>More</span></button>
        </nav>
        <div className="sidebar-foot">
          <div className="user-avatar">{initials}</div><p><strong>{session?.user.email ?? "Signed in"}</strong><small>{session?.organization.role ?? "member"} · authenticated</small></p>
          <button aria-label="Open account menu" aria-expanded={accountOpen} onClick={() => setAccountOpen((open) => !open)}>•••</button>
          {accountOpen ? <div className="account-menu" role="menu" aria-label="Account"><Link role="menuitem" href="/settings" onClick={() => setAccountOpen(false)}>Project settings</Link><button role="menuitem" onClick={() => void signOut()}>Sign out</button></div> : null}
        </div>
      </aside>

      <section className="app-main">
        <header className="app-topbar">
          <div className="mobile-brand"><BrandMark compact /></div>
          <Link href="/environments" className="environment-select"><span className="environment-dot" />{selectedEnvironment} <Icon name="chevron" /></Link>
          <button className="command-trigger" onClick={() => setCommandOpen(true)} aria-haspopup="dialog"><Icon name="command" /><span>Search or run a command</span><kbd>⌘ K</kbd></button>
          <div className="app-topbar__right">
            <StatusPill tone={engineOnline ? "verified" : "neutral"}>{engineOnline ? "Engine online" : "Seeded preview"}</StatusPill>
            <Link className="notification-button" aria-label={`Notifications: ${unreadNotifications} unread`} href="/notifications">{unreadNotifications ? <span /> : null}</Link>
            <button className="mobile-account-button" aria-label="Account menu" aria-expanded={accountOpen} onClick={() => setAccountOpen((open) => !open)}>{initials}</button>
            {accountOpen ? <div className="mobile-account-menu" role="menu" aria-label="Account"><Link role="menuitem" href="/settings" onClick={() => setAccountOpen(false)}>Project settings</Link><button role="menuitem" onClick={() => void signOut()}>Sign out</button></div> : null}
          </div>
        </header>
        {signOutError ? <div className="shell-alert" role="alert"><span>{signOutError}</span><button aria-label="Dismiss sign out error" onClick={() => setSignOutError("")}>×</button></div> : null}
        <div id="product-content">{children}</div>
      </section>

      {commandOpen ? (
        <div className="command-backdrop" role="presentation" onMouseDown={() => setCommandOpen(false)}>
          <div className="command-dialog" role="dialog" aria-modal="true" aria-label="Command palette" onMouseDown={(event) => event.stopPropagation()}>
            <label><Icon name="command" /><span className="sr-only">Search commands</span><input ref={inputRef} value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search scenarios, findings, reports…" /></label>
            <div className="command-results"><span>{results.length ? "Commands" : "No matching commands"}</span>{results.map((item) => <button key={item.href} onClick={() => runCommand(item.href)}><Icon name={item.icon}/>{item.label}<kbd>↵</kbd></button>)}</div>
          </div>
        </div>
      ) : null}
      {moreOpen ? <div className="mobile-more-menu" role="dialog" aria-modal="true" aria-label="More product navigation"><div className="mobile-more-menu__heading"><strong>Product</strong><button aria-label="Close more navigation" onClick={() => setMoreOpen(false)}>×</button></div><nav aria-label="More destinations">{nav.slice(3).map((item) => <Link key={item.href} href={item.href} onClick={() => setMoreOpen(false)}><Icon name={item.icon}/><span>{item.label}</span><Icon name="arrow"/></Link>)}<Link href="/notifications" onClick={() => setMoreOpen(false)}><Icon name="event"/><span>Notifications{unreadNotifications ? ` (${unreadNotifications})` : ""}</span><Icon name="arrow"/></Link></nav><div className="mobile-more-menu__account"><span className="user-avatar">{initials}</span><span><strong>{session?.user.email}</strong><small>{session?.organization.role ?? "member"} · authenticated</small></span><button onClick={() => void signOut()}>Sign out</button></div></div> : null}
    </main>
  );
}
