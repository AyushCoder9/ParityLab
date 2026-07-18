"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { Icon, StatusPill } from "@paritylab/ui";
import { EventBraid } from "./event-braid";

const verificationSystems = [
  ["Browser", "Checkout submitted"],
  ["API", "Intent idempotent"],
  ["Stripe", "Payment confirmed"],
  ["Webhook", "Signature verified"],
  ["Worker", "Effect deduplicated"],
  ["Database", "State converged"],
] as const;

export function HeroVerificationRail() {
  const [active, setActive] = useState(5);

  useEffect(() => {
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    setActive(0);
    const timer = window.setInterval(() => setActive((current) => (current + 1) % verificationSystems.length), 1100);
    return () => window.clearInterval(timer);
  }, []);

  return (
    <aside className="hero-verification" aria-label="Live verification path through six systems">
      <div className="hero-proof">
        <span className="mono">VERIFICATION SURFACE</span>
        <p>Six systems. One observable truth.</p>
      </div>
      <ol className="verification-rail">
        {verificationSystems.map(([system, event], index) => (
          <li key={system} className={index === active ? "is-active" : index < active || active === verificationSystems.length - 1 ? "is-verified" : ""} aria-current={index === active ? "step" : undefined}>
            <span className="mono">{String(index + 1).padStart(2, "0")}</span>
            <span><strong>{system}</strong><small>{event}</small></span>
            <i aria-hidden="true" />
          </li>
        ))}
      </ol>
      <div className="verification-result">
        <span className="mono">BUSINESS EFFECTS</span>
        <strong><span aria-hidden="true">✓</span> Exactly one</strong>
      </div>
    </aside>
  );
}

export function HeroSignal() {
  const [mode, setMode] = useState<"healthy" | "fault" | "verified">("healthy");

  useEffect(() => {
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const timer = window.setInterval(() => {
      setMode((current) => current === "healthy" ? "fault" : current === "fault" ? "verified" : "healthy");
    }, 3400);
    return () => window.clearInterval(timer);
  }, []);

  return (
    <div className="hero-signal">
      <div className="hero-signal__topline">
        <StatusPill tone={mode === "fault" ? "fault" : "verified"}>{mode === "healthy" ? "Payment in motion" : mode === "fault" ? "Duplicate observed" : "State converged"}</StatusPill>
        <span className="mono">RUN_01J8Z4</span>
      </div>
      <EventBraid mode={mode} />
      <div className="hero-signal__readout" aria-live="polite">
        <span>Browser action</span><strong>→</strong><span>Stripe object</span><strong>→</strong><span>{mode === "fault" ? "Fault isolated" : "Invariant checked"}</span>
      </div>
    </div>
  );
}

export function FaultInjector() {
  const [injected, setInjected] = useState(false);

  return (
    <div className={`fault-injector ${injected ? "fault-injector--active" : ""}`}>
      <div className="fault-copy">
        <span className="chapter-marker">Fault injection / duplicate delivery</span>
        <h2>Success paths are easy. Reality arrives twice.</h2>
        <p>Deliver the same signed event again. ParityLab follows the duplicate from ingress to the business invariant—and proves only one fulfillment happened.</p>
        <button onClick={() => setInjected((value) => !value)} className="inject-button">
          <span>{injected ? "Reset fault" : "Inject duplicate"}</span>
          <span className="inject-button__pulse" aria-hidden="true" />
        </button>
      </div>
      <div className="fault-stage">
        <EventBraid mode={injected ? "fault" : "healthy"} labels={false} />
        <div className="fault-evidence" aria-live="polite">
          <div><span>delivery_attempt</span><strong>{injected ? "02" : "01"}</strong></div>
          <div><span>business_effects</span><strong>01</strong></div>
          <div><span>invariant</span><strong>{injected ? "held" : "watching"}</strong></div>
        </div>
      </div>
    </div>
  );
}

const architecture = [
  ["Ingress", "Signature verified from untouched body"],
  ["Outbox", "Event and publish intent commit together"],
  ["Stream", "Delivery order becomes an input, not an assumption"],
  ["Grader", "Stripe, merchant, and derived state are reconciled"],
];

export function ArchitectureFlow() {
  const [active, setActive] = useState(0);
  return (
    <div className="architecture-flow">
      <div className="architecture-track" role="tablist" aria-label="Reliability architecture">
        {architecture.map(([name], index) => (
          <button key={name} role="tab" aria-selected={active === index} onClick={() => setActive(index)}>
            <span className="architecture-index">{String(index + 1).padStart(2, "0")}</span>
            <span>{name}</span>
          </button>
        ))}
      </div>
      <div className="architecture-detail" role="tabpanel">
        <div className="architecture-orbit" aria-hidden="true"><span /><span /><span /></div>
        <p className="mono">{architecture[active][0].toUpperCase()} · GUARANTEE</p>
        <h3>{architecture[active][1]}</h3>
        <p>Every checkpoint stores evidence with a request ID, precise timing, and a stable assertion result. You can reproduce the exact path instead of interpreting a green status code.</p>
        <Link href="/demo">See it under pressure <Icon name="arrow" /></Link>
      </div>
    </div>
  );
}
