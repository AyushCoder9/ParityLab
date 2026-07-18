package engine

import "github.com/ayushkumarsingh/paritylab/services/api/internal/domain"

func seededScenarios() []domain.Scenario {
	return []domain.Scenario{
		{
			ID: "checkout-duplicate", Name: "Duplicate checkout submission", Category: "idempotency",
			Description: "Proves repeated customer submissions produce one PaymentIntent and one merchant order.",
			DurationMS: 4200, Difficulty: "essential", Recommended: true, EstimatedEventCount: 8,
			SupportedFaults: []domain.Fault{domain.FaultNone, domain.FaultDuplicate},
			Assertions: []string{"one Stripe object", "one merchant order", "stable response replay"},
		},
		{
			ID: "webhook-disorder", Name: "Out-of-order webhooks", Category: "webhooks",
			Description: "Delivers lifecycle events in reverse order and verifies current-state convergence.",
			DurationMS: 5800, Difficulty: "advanced", Recommended: true, EstimatedEventCount: 9,
			SupportedFaults: []domain.Fault{domain.FaultNone, domain.FaultReorder, domain.FaultTamper},
			Assertions: []string{"signature verified", "delivery order independent", "state converged"},
		},
		{
			ID: "endpoint-recovery", Name: "Endpoint outage and recovery", Category: "resilience",
			Description: "Injects a temporary timeout, observes retry behavior, and proves exactly-once business effects.",
			DurationMS: 7200, Difficulty: "advanced", Recommended: false, EstimatedEventCount: 10,
			SupportedFaults: []domain.Fault{domain.FaultNone, domain.FaultTimeout},
			Assertions: []string{"fast acknowledgement", "retry recovered", "no event loss"},
		},
		{
			ID: "subscription-renewal", Name: "Subscription renewal recovery", Category: "subscriptions",
			Description: "Advances a test clock through renewal failure and successful payment recovery.",
			DurationMS: 8600, Difficulty: "advanced", Recommended: false, EstimatedEventCount: 12,
			SupportedFaults: []domain.Fault{domain.FaultNone, domain.FaultReorder, domain.FaultTimeout},
			Assertions: []string{"entitlement state correct", "invoice reconciled", "recovery communicated"},
		},
		{
			ID: "refund-convergence", Name: "Partial refund convergence", Category: "reconciliation",
			Description: "Compares Stripe, webhook-derived, and merchant refund state down to integer minor units.",
			DurationMS: 6100, Difficulty: "intermediate", Recommended: false, EstimatedEventCount: 9,
			SupportedFaults: []domain.Fault{domain.FaultNone, domain.FaultDuplicate, domain.FaultReorder},
			Assertions: []string{"minor units exact", "refund idempotent", "three-way state balanced"},
		},
	}
}
