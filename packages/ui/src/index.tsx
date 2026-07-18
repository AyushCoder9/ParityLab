import type { ButtonHTMLAttributes, ReactNode, SVGProps } from "react";

type IconName =
  | "arrow"
  | "check"
  | "chevron"
  | "command"
  | "dashboard"
  | "event"
  | "finding"
  | "pause"
  | "play"
  | "report"
  | "settings"
  | "simulation"
  | "spark"
  | "team";

const iconPaths: Record<IconName, ReactNode> = {
  arrow: <><path d="M5 12h14"/><path d="m14 7 5 5-5 5"/></>,
  check: <path d="m5 12 4 4L19 6"/>,
  chevron: <path d="m9 18 6-6-6-6"/>,
  command: <><path d="M9 6V5a3 3 0 1 0-3 3h12a3 3 0 1 0-3-3v14a3 3 0 1 0 3-3H6a3 3 0 1 0 3 3Z"/></>,
  dashboard: <><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></>,
  event: <><path d="M4 6h16M4 12h11M4 18h8"/><circle cx="19" cy="12" r="2"/></>,
  finding: <><path d="M12 3 2.8 19h18.4L12 3Z"/><path d="M12 9v4M12 17h.01"/></>,
  pause: <><path d="M9 6v12M15 6v12"/></>,
  play: <path d="m8 5 11 7-11 7V5Z"/>,
  report: <><path d="M6 2h9l4 4v16H6V2Z"/><path d="M14 2v5h5M9 13h6M9 17h6"/></>,
  settings: <><circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.7 1.7 0 0 0 .34 1.88l.06.06-2.83 2.83-.06-.06A1.7 1.7 0 0 0 15 19.4a1.7 1.7 0 0 0-1 .6 1.7 1.7 0 0 0-.4 1.1V21H10v-.1A1.7 1.7 0 0 0 9 19.4a1.7 1.7 0 0 0-1.88.34l-.06.06-2.83-2.83.06-.06A1.7 1.7 0 0 0 4.6 15a1.7 1.7 0 0 0-.6-1 1.7 1.7 0 0 0-1.1-.4H3V10h.1A1.7 1.7 0 0 0 4.6 9a1.7 1.7 0 0 0-.34-1.88l-.06-.06 2.83-2.83.06.06A1.7 1.7 0 0 0 9 4.6a1.7 1.7 0 0 0 1-.6 1.7 1.7 0 0 0 .4-1.1V3H14v.1A1.7 1.7 0 0 0 15 4.6a1.7 1.7 0 0 0 1.88-.34l.06-.06 2.83 2.83-.06.06A1.7 1.7 0 0 0 19.4 9c.38.28.62.7.67 1.17.05.48.05.97 0 1.45A1.7 1.7 0 0 0 19.4 15Z"/></>,
  simulation: <><path d="M4 12h4l2-6 4 12 2-6h4"/></>,
  spark: <><path d="M12 2v5M12 17v5M2 12h5M17 12h5"/><path d="m5 5 3 3M16 16l3 3M19 5l-3 3M8 16l-3 3"/></>,
  team: <><circle cx="9" cy="8" r="3"/><circle cx="17" cy="10" r="2"/><path d="M3 20a6 6 0 0 1 12 0M14 16a4 4 0 0 1 7 3"/></>,
};

export function Icon({ name, ...props }: SVGProps<SVGSVGElement> & { name: IconName }) {
  return (
    <svg aria-hidden="true" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round" {...props}>
      {iconPaths[name]}
    </svg>
  );
}

export function BrandMark({ compact = false }: { compact?: boolean }) {
  return (
    <span className="brand-mark" role="img" aria-label="ParityLab">
      <svg aria-hidden="true" viewBox="0 0 32 32" fill="none">
        <path d="M4 8h9c3 0 5 2.2 5 5s2 5 5 5h5" stroke="currentColor" strokeWidth="2"/>
        <path d="M4 16h7c3 0 5 2.2 5 5s2 5 5 5h7" stroke="currentColor" strokeWidth="2"/>
        <path d="M4 24h5c3 0 5-2.2 5-5s2-5 5-5h9" stroke="currentColor" strokeWidth="2" opacity=".48"/>
        <circle cx="18" cy="13" r="2.5" fill="currentColor"/>
      </svg>
      {!compact && <strong>ParityLab</strong>}
    </span>
  );
}

export function StatusPill({ tone, children }: { tone: "verified" | "fault" | "warning" | "neutral"; children: ReactNode }) {
  return <span className={`status-pill status-pill--${tone}`}><span className="status-dot" />{children}</span>;
}

export function Button({ children, className = "", kind = "primary", ...props }: ButtonHTMLAttributes<HTMLButtonElement> & { kind?: "primary" | "secondary" | "quiet" }) {
  return <button className={`button button--${kind} ${className}`} {...props}>{children}</button>;
}
