"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { BrandMark, Button, Icon, StatusPill } from "@paritylab/ui";
import { overview, scenarios } from "@/lib/simulation";
import { EventBraid } from "./event-braid";
import { checkEngine } from "@/lib/api";

const nav = [
  ["Overview", "dashboard"], ["Simulations", "simulation"], ["Events", "event"], ["Findings", "finding"], ["Reports", "report"], ["Team & billing", "team"], ["Settings", "settings"],
] as const;

export function Dashboard() {
  const [active, setActive] = useState("Overview");
  const [commandOpen, setCommandOpen] = useState(false);
  const [engineOnline, setEngineOnline] = useState(false);

  useEffect(() => {
    const controller = new AbortController();
    checkEngine(controller.signal).then(setEngineOnline).catch(() => setEngineOnline(false));
    return () => controller.abort();
  }, []);

  return (
    <main className="app-shell">
      <aside className="app-sidebar">
        <Link href="/" className="brand-link"><BrandMark /></Link>
        <div className="workspace-switcher"><span>PL</span><p><strong>Platform team</strong><small>Sandbox</small></p><Icon name="chevron" /></div>
        <nav aria-label="Product navigation">
          {nav.map(([label, icon], index) => (
            <button key={label} className={active === label ? "is-active" : ""} onClick={() => setActive(label)}>
              <Icon name={icon} /><span>{label}</span>{label === "Findings" && <em>2</em>}{index === 4 && <i />}
            </button>
          ))}
        </nav>
        <div className="sidebar-foot">
          <div className="user-avatar">AK</div><p><strong>Ayush Kumar</strong><small>Owner</small></p><button aria-label="Account menu">•••</button>
        </div>
      </aside>

      <section className="app-main">
        <header className="app-topbar">
          <div className="mobile-brand"><BrandMark compact /></div>
          <div className="environment-select"><span className="environment-dot" />Development <Icon name="chevron" /></div>
          <button className="command-trigger" onClick={() => setCommandOpen(true)}><Icon name="command" /><span>Search or run a command</span><kbd>⌘ K</kbd></button>
          <div className="app-topbar__right"><StatusPill tone={engineOnline ? "verified" : "neutral"}>{engineOnline ? "Engine online" : "Seeded data"}</StatusPill><button className="notification-button" aria-label="Notifications"><span /></button></div>
        </header>

        {active === "Overview" ? <OverviewContent /> : <SectionPlaceholder section={active} onReturn={() => setActive("Overview")} />}
      </section>

      {commandOpen && (
        <div className="command-backdrop" role="presentation" onMouseDown={() => setCommandOpen(false)}>
          <div className="command-dialog" role="dialog" aria-modal="true" aria-label="Command palette" onMouseDown={(event) => event.stopPropagation()}>
            <label><Icon name="command" /><span className="sr-only">Search commands</span><input autoFocus placeholder="Search scenarios, events, findings…" /></label>
            <div className="command-results"><span>Suggested</span>{["Run duplicate webhook simulation", "Open latest critical finding", "Connect Stripe Sandbox"].map((item, index) => <button key={item} onClick={() => setCommandOpen(false)}><Icon name={index === 0 ? "simulation" : index === 1 ? "finding" : "spark"}/>{item}<kbd>↵</kbd></button>)}</div>
          </div>
        </div>
      )}
    </main>
  );
}

function OverviewContent() {
  return (
    <div className="overview-page">
      <div className="overview-heading"><div><p>Friday, July 18</p><h1>Integration overview</h1></div><Link href="/demo" className="button button--primary"><Icon name="play" /> Run simulation</Link></div>
      <div className="overview-grid">
        <section className="readiness-panel">
          <div className="panel-heading"><span>Integration readiness</span><StatusPill tone="verified">Healthy</StatusPill></div>
          <div className="readiness-body">
            <div className="readiness-score" style={{ "--score": `${overview.readiness * 3.6}deg` } as React.CSSProperties}><div><strong>{overview.readiness}</strong><span>/ 100</span></div></div>
            <div><h2>Ready for real failure.</h2><p>7 points stronger after the latest webhook retry run.</p><small>Last verified {overview.lastRun}</small></div>
          </div>
          <div className="readiness-breakdown">{[["Checkout", 98], ["Webhooks", 92], ["Subscriptions", 89], ["Reconciliation", 96]].map(([name, value]) => <div key={name}><span>{name}</span><i><b style={{ width: `${value}%` }} /></i><strong>{value}</strong></div>)}</div>
        </section>

        <section className="finding-panel">
          <div className="panel-heading"><span>Latest finding</span><button>View all <Icon name="arrow" /></button></div>
          <div className="finding-signal"><span><Icon name="check" /></span><div><StatusPill tone="verified">Recovered</StatusPill><h2>Endpoint retry converged</h2><p>The first webhook delivery timed out. Attempt 2 was accepted without a duplicate business effect.</p></div></div>
          <dl><div><dt>First attempt</dt><dd>3.04 s · timeout</dd></div><div><dt>Recovery</dt><dd>86 ms · HTTP 200</dd></div><div><dt>Business effects</dt><dd>Exactly 1</dd></div></dl>
          <button className="text-action">Inspect evidence <Icon name="arrow" /></button>
        </section>

        <section className="topology-panel">
          <div className="panel-heading"><span>Live event topology</span><div><span className="live-dot" /> Observing</div></div>
          <EventBraid compact mode="verified" />
          <div className="topology-legend"><span><i />Browser</span><span><i />API</span><span><i />Stripe</span><span><i />Worker</span><strong>p95 184ms</strong></div>
        </section>

        <section className="runs-panel">
          <div className="panel-heading"><span>Recent simulations</span><button>All runs <Icon name="arrow" /></button></div>
          <div className="runs-table" role="table" aria-label="Recent simulations">
            <div role="row" className="runs-head"><span role="columnheader">Scenario</span><span role="columnheader">Result</span><span role="columnheader">Duration</span><span role="columnheader">Started</span></div>
            {scenarios.map((scenario, index) => <div role="row" key={scenario.slug}><span role="cell"><i className={`run-icon run-icon--${index === 2 ? "warning" : "ok"}`}><Icon name={index === 2 ? "finding" : "check"}/></i><strong>{scenario.name}</strong></span><span role="cell"><StatusPill tone={index === 2 ? "warning" : "verified"}>{index === 2 ? "Recovered" : "Passed"}</StatusPill></span><span role="cell" className="mono">{scenario.duration}s</span><span role="cell">{["2m ago", "18m ago", "1h ago", "3h ago"][index]}</span></div>)}
          </div>
        </section>

        <section className="activity-panel">
          <div className="panel-heading"><span>Signal</span><span>Last 24 hours</span></div>
          <div className="activity-chart"><svg viewBox="0 0 500 120" role="img" aria-label="Webhook latency chart for the last 24 hours"><path d="M0 90 C42 84 50 92 88 75 S148 84 184 56 S248 72 284 62 S342 26 380 52 S438 46 500 24"/><path d="M0 107 C62 101 124 105 184 94 S300 105 360 88 S440 92 500 82"/></svg></div>
          <div className="activity-stats"><div><span>Webhook p95</span><strong>184<span>ms</span></strong></div><div><span>Events observed</span><strong>1,842</strong></div><div><span>Lost events</span><strong>0</strong></div></div>
        </section>
      </div>
    </div>
  );
}

function SectionPlaceholder({ section, onReturn }: { section: string; onReturn: () => void }) {
  return <div className="section-placeholder"><div className="placeholder-orbit"><Icon name="spark" /></div><span>Product demo</span><h1>{section}</h1><p>This complete navigation surface is represented in the seeded demo. The overview and guided simulation contain the live interaction model.</p><div><Button onClick={onReturn}>Return to overview</Button><Link href="/demo" className="button button--secondary">Run simulation</Link></div></div>;
}
