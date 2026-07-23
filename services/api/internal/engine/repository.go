package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/domain"
)

var ErrIdempotencyConflict = errors.New("idempotency key reused with different parameters")
var ErrWebhookConflict = errors.New("webhook event id reused with different payload")

// RunBundle is the atomic persistence unit for a completed deterministic run.
// The run, evidence, report, idempotency response, and outbox message must either
// all commit or all roll back.
type RunBundle struct {
	Run                 domain.Run
	Events              []domain.Event
	Report              domain.Report
	OutboxTopic         string
	StripeCorrelationID string
}

// WebhookReceipt deliberately contains only the minimum metadata needed for
// durable deduplication. Raw payment payloads and credentials are excluded.
type WebhookReceipt struct {
	EventID          string
	EventType        string
	EndpointTokenSHA [sha256.Size]byte
	BodySHA          [sha256.Size]byte
	StripeCreatedAt  time.Time
	StripeObjectID   string
	ObjectStatus     string
	CorrelationID    string
}

type WebhookProcessingStatus string

const (
	WebhookPending   WebhookProcessingStatus = "pending"
	WebhookMatched   WebhookProcessingStatus = "matched"
	WebhookUnmatched WebhookProcessingStatus = "unmatched"
	WebhookIgnored   WebhookProcessingStatus = "ignored"
)

type WebhookProcessingResult struct {
	EventID          string
	EventType        string
	Status           WebhookProcessingStatus
	RunID            string
	ProjectID        string
	ProcessingCode   string
	AlreadyProcessed bool
}

type EventStreamBatch struct {
	Run       domain.Run
	Events    []domain.Event
	HighWater int
}

// BuildWebhookRunEvent creates the sanitized evidence projected into the
// existing run event API after a durable correlation succeeds.
func BuildWebhookRunEvent(receipt WebhookReceipt, runID string, sequence int) domain.Event {
	digest := sha256.Sum256([]byte(receipt.EventID))
	occurredAt := receipt.StripeCreatedAt
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	return domain.Event{
		ID:         "stripe_webhook_" + hex.EncodeToString(digest[:12]),
		RunID:      runID,
		Sequence:   sequence,
		At:         occurredAt,
		Source:     "stripe",
		Target:     "webhook-ingress",
		Type:       receipt.EventType,
		Title:      "Stripe webhook correlated",
		Detail:     "A verified sandbox event was correlated to this run using sanitized Stripe metadata.",
		Status:     domain.EventHealthy,
		Checkpoint: "stripe-webhook",
		TraceID:    "stripe-event-" + hex.EncodeToString(digest[:8]),
		Evidence: map[string]any{
			"stripe_event_id":              receipt.EventID,
			"stripe_event_type":            receipt.EventType,
			"stripe_payment_intent_id":     receipt.StripeObjectID,
			"stripe_payment_intent_status": receipt.ObjectStatus,
			"paritylab_correlation_id":     receipt.CorrelationID,
		},
	}
}

// WebhookCorrelationAssertion is intentionally status-neutral so delivery
// order cannot change the report verdict.
func WebhookCorrelationAssertion(receipt WebhookReceipt) domain.Assertion {
	return domain.Assertion{
		ID:       "assert_stripe_webhook_correlated",
		Name:     "Verified Stripe webhook correlated to the originating run",
		Passed:   true,
		Expected: "matching PaymentIntent and ParityLab correlation metadata",
		Observed: "PaymentIntent and correlation metadata matched",
		Evidence: fmt.Sprintf("stripe_object:%s", receipt.StripeObjectID),
	}
}

// HasAssertion reports whether an assertion projection is already present.
func HasAssertion(assertions []domain.Assertion, id string) bool {
	for _, assertion := range assertions {
		if assertion.ID == id {
			return true
		}
	}
	return false
}

func IsSupportedStripeWebhookType(eventType string) bool {
	switch eventType {
	case "payment_intent.created",
		"payment_intent.processing",
		"payment_intent.succeeded",
		"payment_intent.payment_failed",
		"payment_intent.canceled":
		return true
	default:
		return false
	}
}

type StripeConnection struct {
	ID                    string    `json:"id"`
	StripeAccountID       string    `json:"stripe_account_id"`
	SandboxName           string    `json:"sandbox_name"`
	Status                string    `json:"status"`
	CreatedAt             time.Time `json:"created_at"`
	SecretCiphertext      []byte    `json:"-"`
	SecretEncryptionKeyID int       `json:"-"`
}

type OutboxMessage struct {
	ID          string
	Topic       string
	AggregateID string
	Payload     json.RawMessage
	Attempts    int
	LockedBy    string
	LockedAt    time.Time
}

type VerificationEvidence struct {
	RunID      string
	Assertion  domain.Assertion
	Checkpoint string
}

// Repository is the engine's persistence port. Implementations must make
// CreateRun and MarkWebhookSeen concurrency-safe across processes.
type Repository interface {
	NextRunID(context.Context) (string, error)
	ReplayRun(context.Context, [sha256.Size]byte, [sha256.Size]byte) (domain.Run, bool, error)
	CreateRun(context.Context, [sha256.Size]byte, [sha256.Size]byte, RunBundle) (domain.Run, bool, error)
	Run(context.Context, string) (domain.Run, bool, error)
	Events(context.Context, string) ([]domain.Event, bool, error)
	EventsAfter(context.Context, string, int, int) (EventStreamBatch, bool, error)
	Report(context.Context, string) (domain.Report, bool, error)
	ListRuns(context.Context) ([]domain.Run, error)
	MarkWebhookSeen(context.Context, WebhookReceipt) (bool, error)
	ProcessStripeWebhook(context.Context, string) (WebhookProcessingResult, error)
	SaveStripeConnection(context.Context, StripeConnection) (StripeConnection, error)
	StripeConnection(context.Context, string) (StripeConnection, bool, error)
	ClaimOutbox(context.Context, string, time.Duration, []string) (OutboxMessage, bool, error)
	HeartbeatOutbox(context.Context, string, string, time.Duration) (bool, error)
	CompleteOutbox(context.Context, string, string) error
	RetryOutbox(context.Context, string, string, time.Duration, string) error
	FailOutbox(context.Context, string, string, string) error
	RecordVerification(context.Context, VerificationEvidence) error
	ApplyReferenceMerchantEffect(context.Context, string, int) (bool, error)
	Close() error
}

// TenantRepository is the authenticated product data boundary. Public demo
// records deliberately have a NULL project_id and remain reachable only
// through the legacy Repository reads. Product handlers must use these methods
// after resolving a session and must never accept a project ID from the client.
type TenantRepository interface {
	ReplayRunForProject(context.Context, string, [sha256.Size]byte, [sha256.Size]byte) (domain.Run, bool, error)
	CreateRunForProject(context.Context, string, [sha256.Size]byte, [sha256.Size]byte, RunBundle) (domain.Run, bool, error)
	RunForProject(context.Context, string, string) (domain.Run, bool, error)
	EventsForProject(context.Context, string, string) ([]domain.Event, bool, error)
	EventsAfterForProject(context.Context, string, string, int, int) (EventStreamBatch, bool, error)
	ReportForProject(context.Context, string, string) (domain.Report, bool, error)
	ListRunsForProject(context.Context, string) ([]domain.Run, error)
	SaveStripeConnectionForProject(context.Context, string, StripeConnection) (StripeConnection, error)
	StripeConnectionForProject(context.Context, string, string) (StripeConnection, bool, error)
	ListStripeConnectionsForProject(context.Context, string) ([]StripeConnection, error)
}

type PublicRepository interface {
	PublicRun(context.Context, string) (domain.Run, bool, error)
	PublicEvents(context.Context, string) ([]domain.Event, bool, error)
	PublicEventsAfter(context.Context, string, int, int) (EventStreamBatch, bool, error)
	PublicReport(context.Context, string) (domain.Report, bool, error)
	ListPublicRuns(context.Context) ([]domain.Run, error)
}
