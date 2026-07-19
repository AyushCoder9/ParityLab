package engine

import (
	"fmt"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/domain"
)

type eventSeed struct {
	offset     int
	source     string
	target     string
	typeName   string
	title      string
	detail     string
	status     domain.EventStatus
	latency    int
	checkpoint string
	duplicate  bool
}

func buildEvents(runID string, fault domain.Fault, started time.Time) []domain.Event {
	seeds := []eventSeed{
		{0, "browser", "merchant-api", "checkout.submitted", "Checkout submitted", "The customer confirms a sandbox payment.", domain.EventHealthy, 34, "request", false},
		{420, "merchant-api", "stripe", "payment_intent.created", "PaymentIntent created", "Stripe accepted the idempotent create request.", domain.EventHealthy, 118, "stripe-object", false},
		{980, "stripe", "customer", "payment_intent.succeeded", "Payment confirmed", "The sandbox payment reached a terminal success state.", domain.EventHealthy, 146, "payment", false},
		{1460, "stripe", "webhook-ingress", "webhook.delivered", "Webhook delivered", "The signature is verified against the untouched request body.", domain.EventHealthy, 82, "signature", false},
		{1840, "webhook-ingress", "outbox", "event.persisted", "Event persisted", "Raw metadata and the transactional outbox record commit together.", domain.EventHealthy, 24, "durability", false},
	}

	switch fault {
	case domain.FaultDuplicate:
		seeds = append(seeds,
			eventSeed{2260, "stripe", "webhook-ingress", "webhook.duplicate", "Duplicate delivery detected", "The event identifier already exists; no second business effect is created.", domain.EventDiverged, 76, "deduplication", true},
			eventSeed{2680, "webhook-ingress", "worker", "duplicate.suppressed", "Duplicate suppressed", "The consumer checkpoint advances without replaying the order mutation.", domain.EventRecovered, 31, "deduplication", true},
		)
	case domain.FaultReorder:
		seeds = append(seeds,
			eventSeed{2260, "stripe", "worker", "webhook.reordered", "Older event arrived late", "An earlier lifecycle snapshot arrives after the terminal event.", domain.EventDiverged, 92, "ordering", false},
			eventSeed{2840, "worker", "stripe", "state.hydrated", "Current state hydrated", "The worker fetches the current Stripe object instead of trusting delivery order.", domain.EventRecovered, 134, "ordering", false},
		)
	case domain.FaultTimeout:
		seeds = append(seeds,
			eventSeed{2260, "stripe", "webhook-ingress", "endpoint.timeout", "Endpoint timed out", "The first delivery exceeds the injected response deadline.", domain.EventDiverged, 500, "delivery", false},
			eventSeed{3260, "stripe", "webhook-ingress", "webhook.retried", "Delivery retried", "The retry enters the same durable idempotent pipeline.", domain.EventRecovered, 88, "delivery", false},
		)
	case domain.FaultTamper:
		seeds = append(seeds,
			eventSeed{2260, "stripe", "webhook-ingress", "signature.invalid", "Tampered payload blocked", "The computed signature does not match the signed payload.", domain.EventBlocked, 12, "signature", false},
		)
	}

	if fault != domain.FaultTamper {
		seeds = append(seeds,
			eventSeed{3720, "worker", "merchant-db", "order.reconciled", "Merchant state reconciled", "Stripe, webhook-derived, and merchant state agree.", domain.EventRecovered, 43, "convergence", false},
			eventSeed{4200, "grader", "report", "run.completed", "All invariants verified", "Deterministic assertions produce a passing report.", domain.EventHealthy, 19, "verdict", false},
		)
	}

	events := make([]domain.Event, 0, len(seeds))
	traceID := fmt.Sprintf("trace_%s", runID[4:])
	for i, seed := range seeds {
		events = append(events, domain.Event{
			ID:          fmt.Sprintf("evt_%s_%02d", runID[4:], i+1),
			RunID:       runID,
			Sequence:    i + 1,
			At:          started.Add(time.Duration(seed.offset) * time.Millisecond),
			Source:      seed.source,
			Target:      seed.target,
			Type:        seed.typeName,
			Title:       seed.title,
			Detail:      seed.detail,
			Status:      seed.status,
			LatencyMS:   seed.latency,
			Checkpoint:  seed.checkpoint,
			TraceID:     traceID,
			IsDuplicate: seed.duplicate,
			Evidence: map[string]any{
				"attempt":       1,
				"livemode":      false,
				"api_version":   "2026-06-30.basil",
				"payload_saved": false,
			},
		})
	}
	return events
}

func buildReport(run domain.Run, fault domain.Fault) domain.Report {
	assertions := []domain.Assertion{
		{ID: "assert_stripe_state", Name: "Stripe state reached terminal success", Passed: true, Expected: "succeeded", Observed: "succeeded", Evidence: run.StripeObjectID},
		{ID: "assert_single_effect", Name: "Exactly one merchant business effect", Passed: true, Expected: "1 order", Observed: "1 order", Evidence: run.MerchantOrderID},
		{ID: "assert_convergence", Name: "All state projections converge", Passed: true, Expected: "balanced", Observed: "balanced", Evidence: "stripe = webhook = merchant"},
	}
	findings := []domain.Finding{}
	verdict := "Integration survived the injected condition and converged without data loss."
	if fault != domain.FaultNone {
		findings = append(findings, domain.Finding{
			ID: "finding_" + run.ID[4:], Severity: "info", Title: findingTitle(fault),
			Summary:     "ParityLab observed the injected fault and captured the recovery evidence.",
			Cause:       "A deterministic sandbox fault was inserted at the " + findingCheckpoint(fault) + " checkpoint.",
			Remediation: "No action required. Keep the demonstrated invariant covered in continuous verification.",
			Checkpoint:  findingCheckpoint(fault), Resolved: fault != domain.FaultTamper,
		})
	}
	if fault == domain.FaultTamper {
		assertions[0].Passed = false
		assertions[0].Observed = "blocked before processing"
		assertions[2].Passed = false
		assertions[2].Observed = "not evaluated"
		verdict = "The tampered request was blocked. Downstream convergence was intentionally not evaluated."
		findings[0].Severity = "critical"
		findings[0].Summary = "The signature invariant prevented an untrusted payload from entering the event pipeline."
		findings[0].Remediation = "Rotate any exposed webhook secret and preserve raw-body signature verification."
	}
	return domain.Report{
		Run: run, Summary: fmt.Sprintf("%d of %d deterministic assertions passed.", passedCount(assertions), len(assertions)),
		Verdict: verdict, Assertions: assertions, Findings: findings,
		State:     domain.StateSnapshot{Stripe: "succeeded", Webhook: stateForFault(fault), Merchant: stateForFault(fault), Balanced: fault != domain.FaultTamper},
		Generated: run.CompletedAt.Add(20 * time.Millisecond),
	}
}

func passedCount(assertions []domain.Assertion) int {
	count := 0
	for _, assertion := range assertions {
		if assertion.Passed {
			count++
		}
	}
	return count
}

func stateForFault(fault domain.Fault) string {
	if fault == domain.FaultTamper {
		return "unchanged"
	}
	return "succeeded"
}

func findingTitle(fault domain.Fault) string {
	switch fault {
	case domain.FaultDuplicate:
		return "Duplicate delivery safely suppressed"
	case domain.FaultReorder:
		return "Out-of-order event reconciled"
	case domain.FaultTimeout:
		return "Endpoint recovered on retry"
	case domain.FaultTamper:
		return "Tampered payload rejected"
	default:
		return "Injected condition observed"
	}
}

func findingCheckpoint(fault domain.Fault) string {
	switch fault {
	case domain.FaultDuplicate:
		return "deduplication"
	case domain.FaultReorder:
		return "ordering"
	case domain.FaultTimeout:
		return "delivery"
	case domain.FaultTamper:
		return "signature"
	default:
		return "verdict"
	}
}
