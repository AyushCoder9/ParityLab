"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { Button, BrandMark, Icon, StatusPill } from "@paritylab/ui";
import { getRunState, getVisibleEvents, scenarios } from "@/lib/simulation";
import { EventBraid, EventStatusGlyph } from "./event-braid";
import { createEngineRun } from "@/lib/api";

type DemoMode = "story" | "explore";

const engineScenarios = [
  { scenarioID: "checkout-duplicate", fault: "duplicate" },
  { scenarioID: "webhook-disorder", fault: "reorder" },
  { scenarioID: "endpoint-recovery", fault: "timeout" },
  { scenarioID: "webhook-disorder", fault: "tamper" },
] as const;

export function SimulationConsole() {
  const [mode, setMode] = useState<DemoMode>("story");
  const [scenarioIndex, setScenarioIndex] = useState(0);
  const [progress, setProgress] = useState(0.08);
  const [playing, setPlaying] = useState(true);
  const [speed, setSpeed] = useState(1);
  const [selectedEvent, setSelectedEvent] = useState<string | null>(null);
  const [engineRunID, setEngineRunID] = useState("SEED_PREVIEW");
  const scenario = scenarios[scenarioIndex];
  const runState = getRunState(scenario, progress);
  const visibleEvents = useMemo(() => getVisibleEvents(scenario, progress), [scenario, progress]);
  const activeEvent = visibleEvents.find((event) => event.id === selectedEvent) ?? visibleEvents.at(-1);

  useEffect(() => {
    const requested = new URLSearchParams(window.location.search).get("scenario");
    const requestedIndex = scenarios.findIndex((item) => item.slug === requested);
    if (requestedIndex >= 0) setScenarioIndex(requestedIndex);
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    const engineScenario = engineScenarios[scenarioIndex];
    createEngineRun({
      ...engineScenario,
      idempotencyKey: `paritylab-demo-${scenario.slug}-v1`,
      signal: controller.signal,
    }).then((run) => setEngineRunID(run.id.toUpperCase())).catch(() => setEngineRunID("SEED_PREVIEW"));
    return () => controller.abort();
  }, [scenario.slug, scenarioIndex]);

  useEffect(() => {
    if (!playing) return;
    const timer = window.setInterval(() => {
      setProgress((current) => {
        if (current >= 1) {
          if (mode === "story") {
            window.setTimeout(() => {
              setScenarioIndex((index) => (index + 1) % scenarios.length);
              setProgress(0.02);
            }, 700);
          } else {
            setPlaying(false);
          }
          return 1;
        }
        return Math.min(1, current + 0.006 * speed);
      });
    }, 45);
    return () => window.clearInterval(timer);
  }, [mode, playing, speed]);

  function selectScenario(index: number) {
    setScenarioIndex(index);
    setProgress(0.02);
    setPlaying(true);
    setSelectedEvent(null);
  }

  return (
    <main className="demo-shell">
      <header className="demo-header">
        <Link href="/" className="brand-link"><BrandMark /></Link>
        <div className="mode-switch" role="group" aria-label="Demo mode">
          <button aria-pressed={mode === "story"} onClick={() => setMode("story")}>Story</button>
          <button aria-pressed={mode === "explore"} onClick={() => setMode("explore")}>Explore</button>
        </div>
        <div className="demo-mobile-status"><StatusPill tone="neutral">Simulated data</StatusPill></div>
        <div className="demo-header__right">
          <StatusPill tone="neutral">Simulated data</StatusPill>
          <Link href="/dashboard">Open console <Icon name="arrow" /></Link>
        </div>
      </header>

      <div className="demo-workspace">
        <aside className="scenario-rail" aria-label="Scenarios">
          <div className="scenario-rail__intro">
            <span>Fault library</span><strong>{scenarios.length} seeded</strong>
          </div>
          {scenarios.map((item, index) => (
            <button key={item.slug} className={index === scenarioIndex ? "is-active" : ""} onClick={() => selectScenario(index)} aria-current={index === scenarioIndex ? "true" : undefined}>
              <span className="scenario-number">{String(index + 1).padStart(2, "0")}</span>
              <span><strong>{item.name}</strong><small>{item.summary}</small></span>
              <Icon name="chevron" />
            </button>
          ))}
          <div className="scenario-safe-note"><Icon name="check" /><p><strong>Sandbox safe</strong><span>No live payments or customer data.</span></p></div>
        </aside>

        <section className="simulation-stage" aria-label="Simulation playback">
          <div className="stage-heading">
            <div>
              <span className="mono">{engineRunID} · {scenario.slug}</span>
              <h1>{scenario.name}</h1>
            </div>
            <StatusPill tone={runState === "diverged" ? "fault" : runState === "recovering" ? "warning" : "verified"}>
              {runState === "running" ? "Running" : runState === "diverged" ? "Divergence found" : runState === "recovering" ? "Reconciling" : "Verified"}
            </StatusPill>
          </div>

          <div className={`simulation-viewport simulation-viewport--${runState}`}>
            <div className="viewport-labels mono"><span>BROWSER</span><span>API</span><span>STRIPE</span><span>WEBHOOK</span><span>WORKER</span><span>DATABASE</span></div>
            <EventBraid mode={runState === "diverged" || runState === "recovering" ? "fault" : runState === "verified" ? "verified" : "healthy"} progress={progress} labels={false} />
            <div className="viewport-event" aria-live="polite">
              <span>{activeEvent?.source ?? "System"}</span>
              <strong>{activeEvent?.label ?? "Preparing run"}</strong>
              <small>{activeEvent?.detail ?? "Loading deterministic fixture."}</small>
            </div>
          </div>

          <div className="playback-controls">
            <Button kind="secondary" className="play-button" onClick={() => setPlaying((value) => !value)} aria-label={playing ? "Pause simulation" : "Play simulation"}>
              <Icon name={playing ? "pause" : "play"} /> {playing ? "Pause" : "Play"}
            </Button>
            <label className="timeline-control">
              <span className="sr-only">Simulation progress</span>
              <input type="range" min="0" max="100" value={Math.round(progress * 100)} onChange={(event) => { setProgress(Number(event.target.value) / 100); setPlaying(false); }} />
              <span style={{ width: `${progress * 100}%` }} aria-hidden="true" />
            </label>
            <span className="mono playback-time">{String(Math.round(progress * scenario.duration)).padStart(2, "0")}s / {scenario.duration}s</span>
            <div className="speed-control" role="group" aria-label="Playback speed">
              {[0.5, 1, 2].map((value) => <button key={value} aria-pressed={speed === value} onClick={() => setSpeed(value)}>{value}×</button>)}
            </div>
          </div>

          <div className="simulation-inspector">
            <div className="event-log">
              <div className="panel-title"><span>Event log</span><span>{visibleEvents.length}/{scenario.events.length}</span></div>
              <div className="event-list" role="list" aria-label="Run events">
                {visibleEvents.map((event) => (
                  <button role="listitem" key={event.id} onClick={() => setSelectedEvent(event.id)} className={activeEvent?.id === event.id ? "is-selected" : ""}>
                    <EventStatusGlyph status={event.status} />
                    <span className="mono">{String(event.at).padStart(2, "0")}.0s</span>
                    <span><strong>{event.label}</strong><small>{event.source}</small></span>
                  </button>
                ))}
              </div>
            </div>
            <section className="evidence-panel" aria-label="Evidence">
              <div className="panel-title"><span>Evidence</span><span className="mono">{activeEvent?.id ?? "—"}</span></div>
              <div className="evidence-summary">
                <span className="mono">ASSERTION</span>
                <h2>{activeEvent?.label ?? "Awaiting event"}</h2>
                <p>{activeEvent?.detail ?? "Evidence will appear as the run advances."}</p>
              </div>
              <div className="comparison-grid">
                <div><span>Expected</span><strong>{scenario.expected}</strong></div>
                <div><span>Observed</span><strong>{runState === "diverged" ? "2 deliveries · evaluating effect" : scenario.observed}</strong></div>
              </div>
              <div className="evidence-proof"><Icon name="check"/><span><small>Proof</small><strong>{scenario.evidence}</strong></span></div>
            </section>
          </div>
        </section>
      </div>
    </main>
  );
}
