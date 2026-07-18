# ParityLab project brief

## Why this project exists

The user is preparing for Stripe's Software Engineer Intern role in Bengaluru and wants one flagship side project that demonstrates the qualities in the role rather than a generic payment demo.

Stripe's role emphasizes production software with meaningful impact, systems design and testing, learning unfamiliar systems, technical feedback, cross-functional communication, clear written work, and familiarity with Java, Ruby, JavaScript, Scala, and Go. Recent intern examples included statistics aggregation, service discovery, and clearer Checkout errors.

The project therefore needs to demonstrate:

- A real customer problem tied to Stripe's mission and developer ecosystem.
- Correctness and resilience around money movement.
- Multiple systems and technologies with justified boundaries.
- High-quality testing, code review readiness, documentation, and operational thinking.
- A complete, usable product—not a landing-page mockup or CRUD dashboard.
- A memorable demo that an interviewer can understand quickly.

## User requirements captured from the conversation

The user requested:

- A production-grade, state-of-the-art, highly Stripe-personalized full product.
- Current technologies and languages relevant to Stripe.
- Real features, dashboard, simulation, test cases, audits, repair loops, and a final green signal.
- A jaw-dropping, awards-caliber UI with premium typography, backgrounds, transitions, hover effects, scroll choreography, cards where appropriate, and a unique theme.
- A guided simulation that visibly demonstrates the entire product while remaining connected to real implementation behavior.
- A smooth, highly usable authenticated product—not only a cinematic marketing website.
- Frugal context use, parallel workstreams, durable files describing what is done and what remains, and a fast path into a fresh chat.
- Autonomous continuation without stopping for routine implementation choices.

## Chosen product

**ParityLab** continuously verifies the behavior around a Stripe integration. It injects realistic distributed-system failures—duplicates, event disorder, retries, endpoint outages, tampering, subscription-time changes, concurrency, and API drift—then deterministically checks whether Stripe state, webhook-derived state, and merchant state converge.

The core claim is:

> ParityLab verifies the behavior around Stripe—not merely whether an API request returned 200.

## Audience and jobs to be done

Primary users are payment, platform, backend, and reliability engineers preparing a Stripe integration for production. They need to reproduce failures, understand the exact state transition that broke, and prove that a repair converges safely.

Secondary users are engineering managers, reviewers, interviewers, and stakeholders who need a fast, credible view of readiness and evidence.

## Approved scope

- Test/Sandbox mode only in v1. Live keys and live events must be rejected.
- Seeded deterministic demo requires no external credentials.
- Real Stripe Sandbox verification activates when test credentials are available.
- Public cloud deployment is optional; a complete local product and Stripe Sandbox path define completion.
- Stripe Marketplace review is out of scope because it requires external approval, but a Stripe App extension skeleton is included.

## Success criteria

ParityLab is ready when:

1. Marketing, demo, and dashboard routes build and pass browser/a11y tests.
2. The Go API passes domain, HTTP, idempotency, webhook, redaction, and contract tests.
3. A seeded scenario visually progresses from payment action through fault, deterministic assertions, evidence, and convergence.
4. The API supports the documented overview/scenario/run/SSE/report/webhook contract.
5. Security and operational documentation match implementation.
6. Two consecutive fresh verification runs are green, with limitations explicitly recorded.

## Important honesty constraint

The project is designed to maximize evidence of internship readiness. It cannot guarantee a hiring outcome, and documentation must never imply that tests ran when the required tool or credential was unavailable.
