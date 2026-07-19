import Link from "next/link";
import { Icon, StatusPill } from "@paritylab/ui";
import { ArchitectureFlow, FaultInjector, ForensicNarrative, HeroSignal, HeroVerificationRail, MarketingMotion } from "@/components/marketing";
import { SiteHeader } from "@/components/site-header";

export default function HomePage() {
  return (
    <main className="marketing-page">
      <MarketingMotion />
      <section className="hero-section">
        <SiteHeader />
        <div className="hero-grid">
          <div className="hero-copy">
            <p className="hero-kicker"><span /> Continuous verification for Stripe</p>
            <h1>Your integration should survive <em>reality.</em></h1>
            <p className="hero-lede">ParityLab injects the failures payment systems actually face, then proves your Stripe state and merchant state converge.</p>
            <div className="hero-actions">
              <Link href="/demo" className="cta cta--primary" data-magnetic>Run the simulation <Icon name="arrow" /></Link>
              <Link href="/dashboard" className="cta cta--secondary" data-magnetic>Explore the console</Link>
            </div>
          </div>
          <div className="hero-aside">
            <HeroVerificationRail />
          </div>
        </div>
        <HeroSignal />
        <div className="hero-foot">
          <span>Duplicate-safe</span><span>Order-independent</span><span>Evidence attached</span>
          <a href="#failure" aria-label="Scroll to fault injection"><span>Scroll to inject a fault</span><i /></a>
        </div>
      </section>

      <section className="truth-strip" aria-label="Product principles" data-reveal>
        <p>A <strong>200</strong> means Stripe reached your endpoint.</p>
        <p>It does not mean your system is <strong>correct.</strong></p>
      </section>

      <ForensicNarrative />

      <section className="fault-section" id="failure" data-reveal>
        <FaultInjector />
      </section>

      <section className="system-section" id="system" data-reveal>
        <div className="section-intro">
          <p>One event becomes a system.</p>
          <h2>Follow the evidence,<br />not the happy path.</h2>
        </div>
        <ArchitectureFlow />
      </section>

      <section className="suite-section" data-reveal>
        <div className="suite-title">
          <span>Scenario coverage</span>
          <h2>Make failure<br />routine.</h2>
        </div>
        <div className="suite-list">
          {[
            ["Checkout", "Declines · authentication · abandonment", "09"],
            ["Webhooks", "Duplicates · disorder · tampering", "12"],
            ["Subscriptions", "Renewal · recovery · plan changes", "08"],
            ["Reconciliation", "Refunds · disputes · API drift", "07"],
          ].map(([title, detail, count]) => (
            <Link href="/demo" key={title} data-magnetic>
              <span className="suite-count mono">{count}</span>
              <span><strong>{title}</strong><small>{detail}</small></span>
              <Icon name="arrow" />
            </Link>
          ))}
        </div>
      </section>

      <section className="evidence-section" id="evidence" data-reveal>
        <div className="evidence-console">
          <div className="evidence-toolbar">
            <StatusPill tone="verified">Invariant held</StatusPill>
            <span className="mono">req_01J8Z4KQ2</span>
            <span>87 ms</span>
          </div>
          <div className="evidence-grid">
            <div className="evidence-finding">
              <p>What happened</p>
              <h3>A duplicate arrived.<br />Nothing happened twice.</h3>
              <p>The event ID already existed in the ingress ledger. ParityLab suppressed the second fulfillment and reconciled the current PaymentIntent.</p>
            </div>
            <div className="state-diff">
              <span>Expected merchant state</span><code>fulfillments: 1</code>
              <span>Observed merchant state</span><code>fulfillments: 1</code>
              <span>Stripe state</span><code>amount_received: 4200</code>
              <div><Icon name="check" /><strong>Converged</strong></div>
            </div>
          </div>
        </div>
      </section>

      <section className="final-section" data-reveal>
        <div>
          <p>Ready when the payment fails.</p>
          <h2>Ship with evidence.</h2>
        </div>
        <Link href="/demo" className="cta cta--light">Enter ParityLab <Icon name="arrow" /></Link>
      </section>

      <footer className="site-footer">
        <span>ParityLab</span><p>Continuous verification for payment systems.</p><span className="mono">SANDBOX ONLY · 2026</span>
      </footer>
    </main>
  );
}
