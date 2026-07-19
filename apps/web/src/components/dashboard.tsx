"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import type { Overview } from "@paritylab/contracts";
import { Icon, StatusPill } from "@paritylab/ui";
import { getEngineOverview } from "@/lib/api";
import { overview as seededOverview, scenarios } from "@/lib/simulation";
import { seededRuns } from "@/lib/product-data";
import { EventBraid } from "./event-braid";

export function Dashboard() {
  const [liveOverview, setLiveOverview] = useState<Overview | null>(null);
  const [loading, setLoading] = useState(false);
  const [unavailable, setUnavailable] = useState(true);

  useEffect(() => {
    const controller = new AbortController();
    getEngineOverview(controller.signal).then((value) => { setLiveOverview(value); setUnavailable(false); }).catch(() => setUnavailable(true)).finally(() => setLoading(false));
    return () => controller.abort();
  }, []);

  const readiness = liveOverview?.readiness_score ?? seededOverview.readiness;
  const recentRuns = liveOverview?.recent_runs ?? seededRuns;

  return (
    <div className="product-page overview-page">
      <ProductHeading eyebrow="Verification control plane" title="Integration overview" description="Readiness, recent evidence, and the systems participating in your Stripe Sandbox verification." actions={<Link href="/scenarios" className="button button--primary"><Icon name="play" /> Run simulation</Link>} />
      {loading ? <LoadingStrip /> : unavailable ? <DataNotice /> : <div className="data-notice data-notice--live"><span className="live-dot" /><strong>Live API data</strong><span>Updated from the connected sandbox engine.</span></div>}
      <div className="overview-grid">
        <section className="readiness-panel">
          <div className="panel-heading"><span>Integration readiness</span><StatusPill tone="verified">{liveOverview?.grade ?? "Healthy"}</StatusPill></div>
          <div className="readiness-body">
            <div className="readiness-score" style={{ "--score": `${readiness * 3.6}deg` } as React.CSSProperties}><div><strong>{readiness}</strong><span>/ 100</span></div></div>
            <div><h2>Ready for controlled failure.</h2><p>{liveOverview ? `${liveOverview.stats.passed_runs} of ${liveOverview.stats.total_runs} runs passed.` : "Seeded evidence models the complete connected experience."}</p><small>{liveOverview ? `Last verified ${new Date(liveOverview.last_verified_at).toLocaleString()}` : "Seeded preview · no live claim"}</small></div>
          </div>
          <div className="readiness-breakdown">{(liveOverview?.categories ?? [["Checkout", 98], ["Webhooks", 92], ["Subscriptions", 89], ["Reconciliation", 96]].map(([label, score]) => ({ id: String(label), label: String(label), score: Number(score) }))).map(({ id, label, score }) => <div key={id}><span>{label}</span><i><b style={{ width: `${score}%` }} /></i><strong>{score}</strong></div>)}</div>
        </section>

        <section className="finding-panel">
          <div className="panel-heading"><span>Latest finding</span><Link href="/findings">View all <Icon name="arrow" /></Link></div>
          <div className="finding-signal"><span><Icon name="check" /></span><div><StatusPill tone="verified">Recovered</StatusPill><h2>Endpoint retry converged</h2><p>The first delivery timed out. Attempt 2 was accepted without a duplicate business effect.</p></div></div>
          <dl><div><dt>First attempt</dt><dd>3.04 s · timeout</dd></div><div><dt>Recovery</dt><dd>86 ms · HTTP 200</dd></div><div><dt>Business effects</dt><dd>Exactly 1</dd></div></dl>
          <Link className="text-action" href="/findings?status=open">Inspect evidence <Icon name="arrow" /></Link>
        </section>

        <section className="topology-panel">
          <div className="panel-heading"><span>Event topology</span><div><span className="live-dot" /> {liveOverview ? "Observing" : "Seeded trace"}</div></div>
          <EventBraid compact mode="verified" />
          <div className="topology-legend"><span><i />Browser</span><span><i />API</span><span><i />Stripe</span><span><i />Worker</span><strong>p95 {liveOverview?.stats.p95_latency_ms ?? 184}ms</strong></div>
        </section>

        <section className="runs-panel">
          <div className="panel-heading"><span>Recent runs</span><Link href="/runs">All runs <Icon name="arrow" /></Link></div>
          <div className="runs-table" role="table" aria-label="Recent verification runs">
            <div role="row" className="runs-head"><span role="columnheader">Scenario</span><span role="columnheader">Result</span><span role="columnheader">Duration</span><span role="columnheader">Source</span></div>
            {recentRuns.slice(0, 4).map((run) => <Link role="row" href={`/runs/${run.id}`} key={run.id}><span role="cell"><i className="run-icon"><Icon name="check"/></i><strong>{run.scenario_name}</strong></span><span role="cell"><StatusPill tone="verified">{run.recovered ? "Recovered" : "Passed"}</StatusPill></span><span role="cell" className="mono">{Math.round(run.duration_ms / 1000)}s</span><span role="cell">{liveOverview ? "Live" : "Seeded"}</span></Link>)}
          </div>
        </section>

        <section className="activity-panel">
          <div className="panel-heading"><span>Signal</span><span>Last 24 hours</span></div>
          <div className="activity-chart"><svg viewBox="0 0 500 120" role="img" aria-label="Webhook latency chart"><path d="M0 90 C42 84 50 92 88 75 S148 84 184 56 S248 72 284 62 S342 26 380 52 S438 46 500 24"/><path d="M0 107 C62 101 124 105 184 94 S300 105 360 88 S440 92 500 82"/></svg></div>
          <div className="activity-stats"><div><span>Webhook p95</span><strong>{liveOverview?.stats.p95_latency_ms ?? 184}<span>ms</span></strong></div><div><span>Events observed</span><strong>{liveOverview?.stats.events_processed.toLocaleString() ?? "1,842"}</strong></div><div><span>Duplicates caught</span><strong>{liveOverview?.stats.duplicates_caught ?? scenarios.length}</strong></div></div>
        </section>
      </div>
    </div>
  );
}

export function ProductHeading({ eyebrow, title, description, actions }: { eyebrow: string; title: string; description: string; actions?: React.ReactNode }) {
  return <div className="product-heading"><div><p>{eyebrow}</p><h1>{title}</h1><span>{description}</span></div>{actions ? <div className="product-heading__actions">{actions}</div> : null}</div>;
}

export function DataNotice() {
  return <div className="data-notice data-notice--seeded"><Icon name="finding" /><strong>Engine unavailable — showing seeded preview</strong><span>These records demonstrate product behavior and are not live Stripe data.</span></div>;
}

export function LoadingStrip() {
  return <div className="loading-strip" role="status"><span /><span /><span className="sr-only">Loading product data</span></div>;
}
