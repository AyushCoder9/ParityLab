"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import type { Finding, Report, Run, RunEvent, Scenario as EngineScenario } from "@paritylab/contracts";
import { Icon, StatusPill } from "@paritylab/ui";
import { checkEngine, createEngineRun, createStripePaymentIntentRun, getConnections, getEngineReport, getEngineRun, getEngineRunEvents, getEngineRuns, getEngineScenarios, getEnvironments, getFindings, getNotifications, getProjectSettings, markAllNotificationsRead, markNotificationRead, reopenFinding, resolveFinding, selectEnvironment, updateProjectSettings, validateStripeConnection, type Environment, type Notification, type StripeConnectionSummary } from "@/lib/api";
import { scenarios } from "@/lib/simulation";
import { formatRelativeDate, seededRuns } from "@/lib/product-data";
import { DataNotice, LoadingStrip, ProductHeading } from "./dashboard";
import { EventBraid } from "./event-braid";
import { useAuth } from "./auth-context";

const RESOURCE_CHANGE_EVENT = "paritylab:resources-changed";

function announceResourceChange() {
  window.dispatchEvent(new Event(RESOURCE_CHANGE_EVENT));
}

function SeededLabel() {
  return <StatusPill tone="neutral">Seeded preview</StatusPill>;
}

function EmptyState({ title, detail, clear }: { title: string; detail: string; clear?: () => void }) {
  return <div className="empty-state"><Icon name="command" /><h2>{title}</h2><p>{detail}</p>{clear ? <button className="button button--secondary" onClick={clear}>Clear filters</button> : null}</div>;
}

export function ScenariosPage() {
  const router = useRouter();
  const [query, setQuery] = useState("");
  const [category, setCategory] = useState("All");
  const [engineScenarios, setEngineScenarios] = useState<EngineScenario[]>([]);
  const [launching, setLaunching] = useState<string | null>(null);
  const [launchError, setLaunchError] = useState("");
  useEffect(() => { setQuery(new URLSearchParams(window.location.search).get("query") ?? ""); }, []);
  useEffect(() => { const controller = new AbortController(); getEngineScenarios(controller.signal).then((response) => setEngineScenarios(response.data)).catch(() => setEngineScenarios([])); return () => controller.abort(); }, []);
  const filtered = scenarios.filter((scenario) => (category === "All" || (category === "Security" ? scenario.slug.includes("tampered") : !scenario.slug.includes("tampered"))) && `${scenario.name} ${scenario.summary}`.toLowerCase().includes(query.toLowerCase()));
  const launch = async (scenario: EngineScenario) => { setLaunching(scenario.id); setLaunchError(""); try { const fault = scenario.supported_faults.find((item) => item !== "none") ?? "none"; const run = await createEngineRun({ scenarioID: scenario.id, fault, idempotencyKey: `ui-${scenario.id}-${crypto.randomUUID()}` }); router.push(`/runs/${run.id}`); } catch { setLaunchError("Run creation failed. The engine may be unavailable; no live run was created."); } finally { setLaunching(null); } };
  return <div className="product-page"><ProductHeading eyebrow="Scenario library" title="Failure scenarios" description="Choose the failure mode ParityLab should introduce, then observe whether the integration converges." actions={engineScenarios.length ? <StatusPill tone="verified">Live engine catalog</StatusPill> : <SeededLabel />} />{engineScenarios.length ? <div className="data-notice data-notice--live"><span className="live-dot"/><strong>Live scenario catalog</strong><span>Launching creates a persisted engine run.</span></div> : <DataNotice />}
    {launchError ? <div className="inline-error" role="alert">{launchError}</div> : null}
    {engineScenarios.length ? <section className="live-scenario-strip" aria-label="Live engine scenarios"><div className="panel-heading"><span>Available from the engine</span><span>{engineScenarios.length} scenarios</span></div>{engineScenarios.map((scenario) => <article key={scenario.id}><div><span className="mono">{scenario.category} · {Math.round(scenario.duration_ms / 1000)}s</span><h2>{scenario.name}</h2><p>{scenario.description}</p></div><div className="live-scenario__assertions">{scenario.assertions.slice(0, 2).map((assertion) => <span key={assertion}><Icon name="check"/>{assertion}</span>)}</div><button className="button button--primary" disabled={launching === scenario.id} onClick={() => launch(scenario)}>{launching === scenario.id ? "Creating run…" : "Run on engine"}<Icon name="arrow"/></button></article>)}</section> : null}
    <div className="product-toolbar"><label className="search-field"><Icon name="command"/><span className="sr-only">Search scenarios</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search scenarios" /></label><div className="filter-group" aria-label="Scenario category">{["All", "Reliability", "Security"].map((item) => <button key={item} className={category === item ? "is-active" : ""} onClick={() => setCategory(item)}>{item}</button>)}</div></div>
    {filtered.length ? <div className="scenario-catalog">{filtered.map((scenario, index) => <article className="scenario-card" key={scenario.slug}><div className="scenario-card__index mono">0{index + 1}</div><div><StatusPill tone={scenario.slug === "tampered-payload" ? "warning" : "neutral"}>{scenario.slug === "tampered-payload" ? "Security" : "Reliability"}</StatusPill><h2>{scenario.name}</h2><p>{scenario.summary}</p></div><dl><div><dt>Duration</dt><dd>{scenario.duration}s</dd></div><div><dt>Evidence</dt><dd>{scenario.evidence}</dd></div><div><dt>Events</dt><dd>{scenario.events.length}</dd></div></dl><div className="scenario-card__actions"><Link href={`/demo?scenario=${scenario.slug}`} className="button button--primary"><Icon name="play"/> Run seeded simulation</Link><Link href={`/runs?scenario=${scenario.slug}`} className="text-link">View related runs <Icon name="arrow"/></Link></div></article>)}</div> : <EmptyState title="No scenarios match" detail="Try a different scenario name or category." clear={() => { setQuery(""); setCategory("All"); }} />}
  </div>;
}

export function RunsPage() {
  const [runs, setRuns] = useState<Run[]>(seededRuns);
  const [mode, setMode] = useState<"loading" | "live" | "seeded">("seeded");
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("All");
  const [scenarioFilter, setScenarioFilter] = useState("");
  useEffect(() => { setScenarioFilter(new URLSearchParams(window.location.search).get("scenario") ?? ""); }, []);
  useEffect(() => { const controller = new AbortController(); getEngineRuns(controller.signal).then((response) => { setRuns([...response.data, ...seededRuns]); setMode("live"); }).catch(() => { setRuns(seededRuns); setMode("seeded"); }); return () => controller.abort(); }, []);
  const filtered = runs.filter((run) => (!scenarioFilter || run.scenario_id === scenarioFilter) && (status === "All" || (status === "Recovered" ? run.recovered : run.status === status.toLowerCase())) && `${run.scenario_name} ${run.id}`.toLowerCase().includes(query.toLowerCase()));
  return <div className="product-page"><ProductHeading eyebrow="Evidence ledger" title="Verification runs" description="Search and compare persisted runs. Seeded records remain clearly separated from live engine data." actions={<Link className="button button--primary" href="/scenarios"><Icon name="play"/> New run</Link>} />{mode === "loading" ? <LoadingStrip /> : mode === "seeded" ? <DataNotice /> : <div className="data-notice data-notice--live"><span className="live-dot"/><strong>Live run ledger</strong><span>Loaded from the ParityLab API.</span></div>}
    <div className="product-toolbar"><label className="search-field"><Icon name="command"/><span className="sr-only">Search runs</span><input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Search by scenario or run ID" /></label>{scenarioFilter ? <button className="active-filter" onClick={() => setScenarioFilter("")}>Scenario: {scenarioFilter} ×</button> : null}<div className="filter-group" aria-label="Run status">{["All", "Passed", "Recovered"].map((item) => <button key={item} className={status === item ? "is-active" : ""} onClick={() => setStatus(item)}>{item}</button>)}</div></div>
    {filtered.length ? <div className="data-table" aria-label="Verification runs"><div className="data-table__head"><span>Scenario</span><span>Result</span><span>Duration</span><span>Started</span><span>Source</span></div>{filtered.map((run) => <Link href={`/runs/${run.id}`} key={run.id}><span><strong>{run.scenario_name}</strong><small className="mono">{run.id}</small></span><span><StatusPill tone="verified">{run.recovered ? "Recovered" : "Passed"}</StatusPill></span><span className="mono">{(run.duration_ms / 1000).toFixed(0)}s</span><span>{formatRelativeDate(run.started_at)}</span><span>{run.id.startsWith("seed_") ? "Seeded" : "Live"}<Icon name="arrow"/></span></Link>)}</div> : <EmptyState title="No runs found" detail="Adjust the filters or launch a new scenario." clear={() => { setQuery(""); setStatus("All"); setScenarioFilter(""); }} />}
  </div>;
}

function seededEvents(run: Run): RunEvent[] {
  const scenario = scenarios.find((item) => item.slug === run.scenario_id) ?? scenarios[0];
  return scenario.events.map((event, index) => ({ id: event.id, run_id: run.id, sequence: index + 1, at: new Date(new Date(run.started_at).getTime() + event.at * 1000).toISOString(), source: event.source, target: index === scenario.events.length - 1 ? "Merchant state" : scenario.events[index + 1]?.source ?? "Database", type: event.label.toLowerCase().replaceAll(" ", "."), title: event.label, detail: event.detail, status: event.status === "fault" ? "diverged" : index === scenario.events.length - 1 ? "recovered" : "healthy", latency_ms: [14, 82, 126, 184, 86, 41][index % 6], checkpoint: `${event.source} checkpoint`, trace_id: `seed_trace_${index + 1}`, is_duplicate: event.id.includes("dup") }));
}

export function RunDetailPage({ id }: { id: string }) {
  const fallback = seededRuns.find((run) => run.id === id) ?? seededRuns[0];
  const [run, setRun] = useState<Run>(fallback);
  const [events, setEvents] = useState<RunEvent[]>(seededEvents(fallback));
  const [mode, setMode] = useState<"loading" | "live" | "seeded">("seeded");
  const [copied, setCopied] = useState(false);
  useEffect(() => { const controller = new AbortController(); Promise.all([getEngineRun(id, controller.signal), getEngineRunEvents(id, controller.signal)]).then(([liveRun, response]) => { setRun(liveRun); setEvents(response.data); setMode("live"); }).catch(() => setMode("seeded")); return () => controller.abort(); }, [id]);
  const exportEvidence = () => downloadJSON(`${run.id}-evidence.json`, { source: mode, run, events });
  const copyID = async () => { await navigator.clipboard.writeText(run.id); setCopied(true); window.setTimeout(() => setCopied(false), 1500); };
  return <div className="product-page"><Link href="/runs" className="back-link">← Back to runs</Link><ProductHeading eyebrow={run.id} title={run.scenario_name} description="Follow the complete event path, inspect checkpoints, and export the evidence used for the verdict." actions={<><button className="button button--secondary" onClick={copyID}>{copied ? "Copied" : "Copy run ID"}</button><button className="button button--primary" onClick={exportEvidence}><Icon name="report"/> Export JSON</button></>} />{mode === "loading" ? <LoadingStrip /> : mode === "seeded" ? <DataNotice /> : <div className="data-notice data-notice--live"><span className="live-dot"/><strong>Live run evidence</strong><span>Loaded from the engine.</span></div>}
    <div className="run-hero"><div><span>Verdict</span><strong><Icon name="check"/> {run.recovered ? "Recovered safely" : "Invariant passed"}</strong></div><div><span>Score</span><strong>{run.score}/100</strong></div><div><span>Business effects</span><strong>Exactly one</strong></div><div><span>Environment</span><strong>Stripe Sandbox</strong></div></div>
    <section className="product-panel topology-detail"><div className="panel-heading"><span>System topology</span><span className="mono">{events.length} checkpoints</span></div><EventBraid mode="verified" compact /></section>
    <section className="product-panel"><div className="panel-heading"><span>Event ledger</span><span>{mode === "live" ? "Live evidence" : "Seeded evidence"}</span></div><ol className="event-ledger">{events.map((event) => <li key={event.id}><span className={`event-state event-state--${event.status}`} /><time className="mono">{new Date(event.at).toLocaleTimeString()}</time><div><strong>{event.title}</strong><p>{event.detail}</p></div><span>{event.source}</span><code>{event.latency_ms}ms</code></li>)}</ol></section>
  </div>;
}

export function FindingsPage() {
  const [items, setItems] = useState<Finding[]>([]);
  const [filter, setFilter] = useState("Open");
  const [state, setState] = useState<"loading" | "ready" | "error">("loading");
  const [error, setError] = useState(""); const [mutating, setMutating] = useState<string | null>(null);
  useEffect(() => { const controller = new AbortController(); setState("loading"); setError(""); getFindings(filter === "Open" ? "open" : filter === "Resolved" ? "resolved" : "", controller.signal).then((response) => { setItems(response.data); setState("ready"); }).catch((value) => { if (!controller.signal.aborted) { setError(value instanceof Error ? value.message : "Findings could not be loaded."); setState("error"); } }); return () => controller.abort(); }, [filter]);
  const changeResolution = async (finding: Finding) => { setMutating(finding.id); setError(""); try { const updated = finding.resolved ? await reopenFinding(finding.id) : await resolveFinding(finding.id); setItems((current) => filter === "All" ? current.map((item) => item.id === updated.id ? updated : item) : current.filter((item) => item.id !== updated.id)); } catch (value) { setError(value instanceof Error ? value.message : "The finding could not be updated."); } finally { setMutating(null); } };
  return <div className="product-page"><ProductHeading eyebrow="Invariant triage" title="Findings" description="Understand why an invariant diverged, persist remediation state, and rerun the exact failure mode." actions={<StatusPill tone="verified">Persisted triage</StatusPill>} /><div className="data-notice data-notice--live"><span className="live-dot"/><strong>Workspace finding ledger</strong><span>Resolution state is stored by the ParityLab API.</span></div>{error ? <div className="inline-error" role="alert">{error}</div> : null}<div className="product-toolbar"><div className="filter-group">{["Open", "Resolved", "All"].map((item) => <button key={item} className={filter === item ? "is-active" : ""} onClick={() => setFilter(item)}>{item}</button>)}</div></div>{state === "loading" ? <LoadingStrip /> : state === "error" ? <EmptyState title="Findings are unavailable" detail="The API did not return the workspace finding ledger." /> : items.length ? <div className="finding-list">{items.map((finding) => <article key={finding.id}><div className="finding-list__signal"><StatusPill tone={finding.severity === "critical" ? "fault" : finding.severity === "warning" ? "warning" : "neutral"}>{finding.severity}</StatusPill><span className="mono">{finding.checkpoint}</span></div><h2>{finding.title}</h2><p>{finding.summary}</p><dl><div><dt>Cause</dt><dd>{finding.cause}</dd></div><div><dt>Remediation</dt><dd>{finding.remediation}</dd></div></dl><div className="row-actions"><button className="button button--secondary" disabled={mutating === finding.id} onClick={() => void changeResolution(finding)}>{mutating === finding.id ? "Saving…" : finding.resolved ? "Reopen finding" : "Mark resolved"}</button><Link href="/scenarios" className="button button--quiet">Run another scenario <Icon name="arrow"/></Link></div></article>)}</div> : <EmptyState title="No findings in this view" detail="The selected persisted triage queue is clear." clear={() => setFilter("All")} />}</div>;
}

export function ReportsPage() {
  const [runs, setRuns] = useState<Run[]>(seededRuns); const [live, setLive] = useState(false);
  useEffect(() => { const controller = new AbortController(); getEngineRuns(controller.signal).then((response) => { if (response.data.length) { setRuns([...response.data, ...seededRuns]); setLive(true); } }).catch(() => undefined); return () => controller.abort(); }, []);
  return <div className="product-page"><ProductHeading eyebrow="Immutable evidence" title="Reports" description="Open, print, or export the verdict and the assertions that produced it." actions={live ? <StatusPill tone="verified">Live reports available</StatusPill> : <SeededLabel />} />{live ? <div className="data-notice data-notice--live"><span className="live-dot"/><strong>Live reports and seeded examples</strong><span>Each row identifies its source.</span></div> : <DataNotice/>}<div className="report-grid">{runs.map((run) => <Link href={`/reports/${run.id}`} key={run.id}><Icon name="report"/><div><span className="mono">{run.id}</span><h2>{run.scenario_name}</h2><p>{run.recovered ? "Recovered after controlled divergence" : "All assertions passed"}</p></div><StatusPill tone={run.id.startsWith("seed_") ? "neutral" : "verified"}>{run.id.startsWith("seed_") ? "Seeded" : "Live"}</StatusPill><Icon name="arrow"/></Link>)}</div></div>;
}

export function ReportPage({ id }: { id: string }) {
  const run = seededRuns.find((item) => item.id === id) ?? seededRuns[0];
  const [report, setReport] = useState<Report | null>(null);
  const [mode, setMode] = useState<"loading" | "live" | "seeded">("seeded");
  useEffect(() => { const controller = new AbortController(); getEngineReport(id, controller.signal).then((value) => { setReport(value); setMode("live"); }).catch(() => setMode("seeded")); return () => controller.abort(); }, [id]);
  const exportReport = () => downloadJSON(`${id}-report.json`, report ?? { source: "seeded", run, verdict: "Verified", assertions: [{ name: "Exactly one business effect", passed: true }, { name: "Stripe and merchant state converge", passed: true }, { name: "Signature boundary enforced", passed: true }] });
  return <div className="product-page report-page"><Link href="/reports" className="back-link">← Back to reports</Link><ProductHeading eyebrow={`Report · ${id}`} title={report?.run.scenario_name ?? run.scenario_name} description="A reproducible record of the inputs, checkpoints, assertions, and final state." actions={<><button className="button button--secondary" onClick={() => window.print()}>Print</button><button className="button button--primary" onClick={exportReport}><Icon name="report"/> Export JSON</button></>} />{mode === "loading" ? <LoadingStrip /> : mode === "seeded" ? <DataNotice /> : <div className="data-notice data-notice--live"><span className="live-dot"/><strong>Live immutable report</strong></div>}<div className="report-verdict"><Icon name="check"/><div><span>Verdict</span><h2>{report?.verdict ?? "Integration converged safely"}</h2><p>{report?.summary ?? "Every seeded assertion passed after the controlled failure was introduced."}</p></div><strong>{report?.run.score ?? run.score}/100</strong></div><section className="product-panel"><div className="panel-heading"><span>Assertions</span><span>{report?.assertions.length ?? 3} evaluated</span></div><div className="assertion-list">{(report?.assertions ?? [{ id: "a1", name: "Exactly one business effect", passed: true, expected: "1 effect", observed: "1 effect", evidence: "merchant order uniqueness" }, { id: "a2", name: "Stripe and merchant state converge", passed: true, expected: "paid", observed: "paid", evidence: "current state comparison" }, { id: "a3", name: "Signature boundary enforced", passed: true, expected: "valid only", observed: "invalid rejected", evidence: "raw body signature" }]).map((assertion) => <div key={assertion.id}><Icon name="check"/><div><strong>{assertion.name}</strong><span>{assertion.evidence}</span></div><code>{assertion.expected} = {assertion.observed}</code></div>)}</div></section></div>;
}

export function ConnectionsPage() {
  const router = useRouter();
  const [checking, setChecking] = useState(false); const [result, setResult] = useState<"idle" | "online" | "offline">("idle");
  const [secretKey, setSecretKey] = useState(""); const [sandboxName, setSandboxName] = useState("ParityLab internship demo");
  const [connections, setConnections] = useState<StripeConnectionSummary[]>([]);
  const [connection, setConnection] = useState<StripeConnectionSummary | null>(null); const [connectionError, setConnectionError] = useState(""); const [connecting, setConnecting] = useState(false); const [creatingPayment, setCreatingPayment] = useState(false); const [loadingConnections, setLoadingConnections] = useState(true);
  useEffect(() => {
    const controller = new AbortController();
    getConnections(controller.signal)
      .then((response) => {
        setConnections(response.data);
        setConnection(response.data[0] ?? null);
      })
      .catch((value) => {
        if (!controller.signal.aborted) setConnectionError(value instanceof Error ? value.message : "Saved connections could not be loaded.");
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoadingConnections(false);
      });
    return () => controller.abort();
  }, []);
  const testEngine = async () => { setChecking(true); try { setResult(await checkEngine() ? "online" : "offline"); } catch { setResult("offline"); } finally { setChecking(false); } };
  const connect = async (event: React.FormEvent) => { event.preventDefault(); setConnectionError(""); if (secretKey.includes("_live_")) { setConnectionError("Live Stripe keys are blocked. Use a restricted Sandbox key."); setSecretKey(""); return; } setConnecting(true); try { const created = await validateStripeConnection({ secretKey, sandboxName }); setConnection(created); setConnections((current) => [created, ...current.filter((item) => item.id !== created.id)]); announceResourceChange(); } catch (error) { setConnectionError(error instanceof Error ? error.message : "Stripe Sandbox validation failed."); } finally { setSecretKey(""); setConnecting(false); } };
  const runRealPayment = async () => { if (!connection) return; setCreatingPayment(true); setConnectionError(""); try { const run = await createStripePaymentIntentRun({ connectionID: connection.id, amountMinor: 4200, currency: "usd", idempotencyKey: `stripe-ui-${crypto.randomUUID()}` }); router.push(`/runs/${run.id}`); } catch (error) { setConnectionError(error instanceof Error ? error.message : "Stripe Sandbox run failed."); } finally { setCreatingPayment(false); } };
  return <div className="product-page"><ProductHeading eyebrow="Secure integrations" title="Connections" description="Validate a restricted Stripe Sandbox key server-side, then create a real $42.00 test PaymentIntent run." actions={connections.length ? <StatusPill tone="verified">{connections.length} persisted</StatusPill> : undefined} />{connectionError ? <div className="inline-error" role="alert">{connectionError}</div> : null}{loadingConnections ? <LoadingStrip /> : <form className="stripe-connection-form" onSubmit={connect}><div><StatusPill tone={connection ? "verified" : "warning"}>{connection ? "Sandbox connected" : "Secret sent once"}</StatusPill><h2>{connection ? connection.sandbox_name : "Connect Stripe Sandbox"}</h2><p>{connection ? `Connected account ${connection.stripe_account_id}. The browser only received this sanitized connection record.` : "The secret is posted over the configured API connection, never stored in browser storage, never logged by this UI, and cleared from memory immediately after submission."}</p></div>{connection ? <div className="connected-record"><span>Connection ID</span><code>{connection.id}</code><span>Status</span><strong>{connection.status}</strong>{connections.length > 1 ? <label>Active connection<select value={connection.id} onChange={(event) => setConnection(connections.find((item) => item.id === event.target.value) ?? connection)}>{connections.map((item) => <option key={item.id} value={item.id}>{item.sandbox_name}</option>)}</select></label> : null}<button type="button" className="button button--primary" disabled={creatingPayment} onClick={runRealPayment}>{creatingPayment ? "Creating Stripe run…" : "Run real $42 Sandbox payment"}<Icon name="arrow"/></button></div> : <div className="connection-fields"><label>Sandbox name<input value={sandboxName} onChange={(event) => setSandboxName(event.target.value)} required /></label><label>Restricted Sandbox secret key<input type="password" autoComplete="new-password" spellCheck={false} value={secretKey} onChange={(event) => setSecretKey(event.target.value)} placeholder="rk_test_… or sk_test_…" required /></label><button className="button button--primary" disabled={connecting}>{connecting ? "Validating securely…" : "Validate and connect"}<Icon name="arrow"/></button></div>}</form>}<div className="connection-grid"><section className="connection-card"><div className="connection-card__icon"><Icon name="simulation"/></div><div><span>Control plane</span><h2>ParityLab engine</h2><p>Checks the configured API origin and verifies sandbox mode.</p></div><StatusPill tone={result === "online" ? "verified" : result === "offline" ? "fault" : "neutral"}>{result === "online" ? "Connected" : result === "offline" ? "Engine unavailable" : "Not checked"}</StatusPill><button className="button button--secondary" disabled={checking} onClick={testEngine}>{checking ? "Checking…" : "Run connection check"}</button></section><section className="connection-card"><div className="connection-card__icon"><Icon name="spark"/></div><div><span>Payments source</span><h2>Stripe Sandbox</h2><p>{connection ? `Connected as ${connection.stripe_account_id}.` : "Use the secure form above. Live-mode credentials are rejected at both UI and API boundaries."}</p></div><StatusPill tone={connection ? "verified" : "warning"}>{connection ? "Validated" : "Configuration required"}</StatusPill><Link className="button button--secondary" href="/onboarding">Review secure setup</Link></section><section className="connection-card"><div className="connection-card__icon"><Icon name="event"/></div><div><span>Verification target</span><h2>Reference merchant</h2><p>The bundled target is a deterministic local service; no external merchant endpoint is connected.</p></div><StatusPill tone="neutral">Bundled target</StatusPill><Link className="button button--secondary" href="/demo">Inspect contract behavior</Link></section></div></div>;
}

export function EnvironmentsPage() {
  const [items, setItems] = useState<Environment[]>([]);
  const [state, setState] = useState<"loading" | "ready" | "error">("loading");
  const [changing, setChanging] = useState<string | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    getEnvironments(controller.signal)
      .then((response) => { setItems(response.data); setState("ready"); })
      .catch((value) => { if (!controller.signal.aborted) { setError(value instanceof Error ? value.message : "Environments could not be loaded."); setState("error"); } });
    return () => controller.abort();
  }, []);
  const choose = async (environment: Environment) => {
    if (environment.is_default) return;
    setChanging(environment.id); setError("");
    try {
      const selected = await selectEnvironment(environment.id);
      setItems((current) => current.map((item) => ({ ...item, is_default: item.id === selected.id })));
      announceResourceChange();
    } catch (value) {
      setError(value instanceof Error ? value.message : "The environment could not be selected.");
    } finally { setChanging(null); }
  };
  const selected = items.find((item) => item.is_default);
  const details: Record<Environment["kind"], string> = {
    local: "Local fixtures and the bundled reference merchant.",
    sandbox: "Real Stripe test objects and signed sandbox webhooks.",
    staging: "Hosted pre-production isolation for deployment acceptance.",
  };
  return <div className="product-page"><ProductHeading eyebrow="Isolation boundaries" title="Environments" description="Keep local fixtures, Stripe Sandbox evidence, and staging activity visibly separated." actions={<StatusPill tone="verified">Persisted selection</StatusPill>} />{error ? <div className="inline-error" role="alert">{error}</div> : null}{state === "loading" ? <LoadingStrip /> : state === "error" ? <EmptyState title="Environments are unavailable" detail="The API did not return the project environment ledger." /> : <><div className="environment-list">{items.map((environment) => <button key={environment.id} disabled={changing !== null} className={environment.is_default ? "is-selected" : ""} aria-pressed={environment.is_default} onClick={() => void choose(environment)}><span className="environment-radio"/><div><strong>{environment.name}</strong><p>{details[environment.kind]}</p></div><StatusPill tone={environment.is_default ? "verified" : "neutral"}>{changing === environment.id ? "Selecting…" : environment.is_default ? "Active" : environment.kind}</StatusPill></button>)}</div><div className="selection-note" role="status" aria-live="polite"><strong>{selected?.name ?? "No environment"} selected</strong><span>The API stores this selection for the authenticated project.</span></div></>}</div>;
}

export function NotificationsPage() {
  const [items, setItems] = useState<Notification[]>([]);
  const [state, setState] = useState<"loading" | "ready" | "error">("loading");
  const [error, setError] = useState(""); const [mutating, setMutating] = useState(false);
  useEffect(() => {
    const controller = new AbortController();
    getNotifications(controller.signal)
      .then((response) => { setItems(response.data); setState("ready"); })
      .catch((value) => { if (!controller.signal.aborted) { setError(value instanceof Error ? value.message : "Notifications could not be loaded."); setState("error"); } });
    return () => controller.abort();
  }, []);
  const unread = items.filter((item) => !item.read_at).length;
  const read = async (notification: Notification) => {
    if (notification.read_at) return;
    setError("");
    try {
      const updated = await markNotificationRead(notification.id);
      setItems((current) => current.map((item) => item.id === updated.id ? updated : item));
      announceResourceChange();
    } catch (value) { setError(value instanceof Error ? value.message : "The notification could not be updated."); }
  };
  const readAll = async () => {
    setMutating(true); setError("");
    try {
      await markAllNotificationsRead();
      const readAt = new Date().toISOString();
      setItems((current) => current.map((item) => ({ ...item, read_at: item.read_at ?? readAt })));
      announceResourceChange();
    } catch (value) { setError(value instanceof Error ? value.message : "Notifications could not be marked as read."); }
    finally { setMutating(false); }
  };
  return <div className="product-page"><ProductHeading eyebrow="Run activity" title="Notifications" description="Track persisted run completion, divergence, and report availability." actions={unread ? <button className="button button--secondary" disabled={mutating} onClick={() => void readAll()}>{mutating ? "Saving…" : `Mark all read (${unread})`}</button> : <StatusPill tone="verified">All read</StatusPill>} />{error ? <div className="inline-error" role="alert">{error}</div> : null}{state === "loading" ? <LoadingStrip /> : state === "error" ? <EmptyState title="Notifications are unavailable" detail="The API did not return the project activity ledger." /> : items.length ? <><div className="data-notice data-notice--live"><span className="live-dot"/><strong>Persisted project activity</strong><span>Read state survives browser and API restarts.</span></div><div className="notification-list">{items.map((item) => { const title = typeof item.payload.title === "string" ? item.payload.title : item.kind.replaceAll(".", " "); const detail = typeof item.payload.detail === "string" ? item.payload.detail : item.run_id ? `Run ${item.run_id}` : "Project activity recorded by ParityLab."; return <button key={item.id} className={!item.read_at ? "is-unread" : ""} disabled={Boolean(item.read_at)} onClick={() => void read(item)}><span className="notification-state"/><div><strong>{title}</strong><p>{detail}</p></div><time dateTime={item.created_at}>{formatRelativeDate(item.created_at)}</time></button>; })}</div></> : <EmptyState title="No notifications yet" detail="Run activity and evidence updates will appear here." />}</div>;
}

export function SettingsPage() {
  const { refresh, session } = useAuth();
  const [saved, setSaved] = useState(false); const [name, setName] = useState(session?.project.name ?? ""); const [retention, setRetention] = useState(String(session?.project.retention_days ?? 30));
  const [state, setState] = useState<"loading" | "ready" | "error">("loading"); const [saving, setSaving] = useState(false); const [error, setError] = useState("");
  useEffect(() => {
    const controller = new AbortController();
    getProjectSettings(controller.signal)
      .then((settings) => { setName(settings.name); setRetention(String(settings.retention_days)); setState("ready"); })
      .catch((value) => { if (!controller.signal.aborted) { setError(value instanceof Error ? value.message : "Project settings could not be loaded."); setState("error"); } });
    return () => controller.abort();
  }, []);
  const save = async (event: React.FormEvent) => {
    event.preventDefault(); setSaving(true); setSaved(false); setError("");
    try {
      const updated = await updateProjectSettings({ name: name.trim(), retention_days: Number(retention) });
      setName(updated.name); setRetention(String(updated.retention_days)); setSaved(true);
      await refresh();
      announceResourceChange();
    } catch (value) { setError(value instanceof Error ? value.message : "Project settings could not be saved."); }
    finally { setSaving(false); }
  };
  return <div className="product-page"><ProductHeading eyebrow="Workspace controls" title="Settings" description="Manage the authenticated project and its persisted evidence policy." actions={<StatusPill tone="verified">{session?.organization.role ?? "member"}</StatusPill>} />{error ? <div className="inline-error" role="alert">{error}</div> : null}{state === "loading" ? <LoadingStrip /> : state === "error" ? <EmptyState title="Settings are unavailable" detail="The API did not return the authenticated project settings." /> : <form className="settings-form" onSubmit={(event) => void save(event)}><section><div><h2>Project</h2><p>Used across this organization&apos;s verification workspace.</p></div><label>Project name<input required value={name} onChange={(event) => { setName(event.target.value); setSaved(false); }} /></label><label>Evidence retention<select value={retention} onChange={(event) => { setRetention(event.target.value); setSaved(false); }}><option value="7">7 days</option><option value="30">30 days</option><option value="90">90 days</option></select></label></section><section><div><h2>Safety boundary</h2><p>ParityLab only accepts Stripe Sandbox data.</p></div><div className="safety-lock"><Icon name="check"/><span><strong>Live mode blocked</strong><small>sk_live_, rk_live_, pk_live_, and livemode events are rejected.</small></span></div></section><div className="settings-actions"><span role="status" aria-live="polite">{saved ? "Project settings saved to ParityLab." : "Changes are persisted when you save."}</span><button className="button button--primary" type="submit" disabled={saving}>{saving ? "Saving…" : "Save changes"}</button></div></form>}</div>;
}

function downloadJSON(filename: string, value: unknown) {
  const blob = new Blob([JSON.stringify(value, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob); const anchor = document.createElement("a"); anchor.href = url; anchor.download = filename; anchor.click(); URL.revokeObjectURL(url);
}
