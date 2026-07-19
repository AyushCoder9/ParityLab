package engine

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/domain"
)

type memoryIdempotencyRecord struct {
	requestHash [sha256.Size]byte
	runID       string
}

// MemoryRepository is the credential-free local adapter. It preserves the
// deterministic demo while exercising the same port as PostgreSQL.
type MemoryRepository struct {
	mu              sync.RWMutex
	runs            map[string]domain.Run
	events          map[string][]domain.Event
	reports         map[string]domain.Report
	idempotency     map[[sha256.Size]byte]memoryIdempotencyRecord
	webhooks        map[string]WebhookReceipt
	connections     map[string]StripeConnection
	nextRun         int
	outbox          map[string]memoryOutbox
	nextOutbox      int
	merchantEffects map[string]int
}

type memoryOutbox struct {
	message     OutboxMessage
	availableAt time.Time
	published   bool
	failed      bool
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		runs: make(map[string]domain.Run), events: make(map[string][]domain.Event),
		reports: make(map[string]domain.Report), idempotency: make(map[[sha256.Size]byte]memoryIdempotencyRecord),
		webhooks: make(map[string]WebhookReceipt), connections: make(map[string]StripeConnection), nextRun: 4,
		outbox: make(map[string]memoryOutbox), nextOutbox: 1, merchantEffects: make(map[string]int),
	}
}

func (r *MemoryRepository) NextRunID(context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := runID(r.nextRun)
	r.nextRun++
	return id, nil
}

func (r *MemoryRepository) ReplayRun(_ context.Context, keyHash, requestHash [sha256.Size]byte) (domain.Run, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	previous, exists := r.idempotency[keyHash]
	if !exists {
		return domain.Run{}, false, nil
	}
	if previous.requestHash != requestHash {
		return domain.Run{}, false, ErrIdempotencyConflict
	}
	return r.runs[previous.runID], true, nil
}

func (r *MemoryRepository) CreateRun(_ context.Context, keyHash, requestHash [sha256.Size]byte, bundle RunBundle) (domain.Run, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if previous, exists := r.idempotency[keyHash]; exists {
		if previous.requestHash != requestHash {
			return domain.Run{}, false, ErrIdempotencyConflict
		}
		return r.runs[previous.runID], true, nil
	}
	r.runs[bundle.Run.ID] = bundle.Run
	r.events[bundle.Run.ID] = cloneEvents(bundle.Events)
	r.reports[bundle.Run.ID] = bundle.Report
	r.idempotency[keyHash] = memoryIdempotencyRecord{requestHash: requestHash, runID: bundle.Run.ID}
	r.enqueueLocked(bundle)
	return bundle.Run, false, nil
}

func (r *MemoryRepository) putSeed(bundle RunBundle) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runs[bundle.Run.ID] = bundle.Run
	r.events[bundle.Run.ID] = cloneEvents(bundle.Events)
	r.reports[bundle.Run.ID] = bundle.Report
}

func (r *MemoryRepository) Run(_ context.Context, id string) (domain.Run, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.runs[id]
	return value, ok, nil
}

func (r *MemoryRepository) Events(_ context.Context, id string) ([]domain.Event, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.events[id]
	return cloneEvents(value), ok, nil
}

func (r *MemoryRepository) Report(_ context.Context, id string) (domain.Report, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.reports[id]
	return value, ok, nil
}

func (r *MemoryRepository) ListRuns(context.Context) ([]domain.Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]domain.Run, 0, len(r.runs))
	for _, value := range r.runs {
		items = append(items, value)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].StartedAt.After(items[j].StartedAt) })
	return items, nil
}

func (r *MemoryRepository) MarkWebhookSeen(_ context.Context, receipt WebhookReceipt) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if previous, exists := r.webhooks[receipt.EventID]; exists {
		if previous.EventType != receipt.EventType || previous.EndpointTokenSHA != receipt.EndpointTokenSHA || previous.BodySHA != receipt.BodySHA {
			return false, ErrWebhookConflict
		}
		return true, nil
	}
	r.webhooks[receipt.EventID] = receipt
	payload, _ := json.Marshal(map[string]string{"stripe_event_id": receipt.EventID, "event_type": receipt.EventType})
	r.putOutboxLocked("stripe.webhook.received", receipt.EventID, payload)
	return false, nil
}

func (r *MemoryRepository) Close() error { return nil }

func (r *MemoryRepository) SaveStripeConnection(_ context.Context, connection StripeConnection) (StripeConnection, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, existing := range r.connections {
		if existing.StripeAccountID == connection.StripeAccountID {
			connection.ID = id
			connection.CreatedAt = existing.CreatedAt
		}
	}
	connection.SecretCiphertext = append([]byte(nil), connection.SecretCiphertext...)
	r.connections[connection.ID] = connection
	return connection, nil
}

func (r *MemoryRepository) StripeConnection(_ context.Context, id string) (StripeConnection, bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	connection, ok := r.connections[id]
	connection.SecretCiphertext = append([]byte(nil), connection.SecretCiphertext...)
	return connection, ok, nil
}

func cloneEvents(events []domain.Event) []domain.Event {
	return append([]domain.Event(nil), events...)
}

func (r *MemoryRepository) enqueueLocked(bundle RunBundle) {
	topic := bundle.OutboxTopic
	if topic == "" {
		topic = "run.persisted"
	}
	payload, _ := json.Marshal(map[string]any{
		"run_id": bundle.Run.ID, "scenario_id": bundle.Run.ScenarioID,
		"status": bundle.Run.Status, "environment": bundle.Run.Environment,
	})
	r.putOutboxLocked(topic, bundle.Run.ID, payload)
}

func (r *MemoryRepository) putOutboxLocked(topic, aggregateID string, payload []byte) {
	id := fmt.Sprintf("outbox_%06d", r.nextOutbox)
	r.nextOutbox++
	r.outbox[id] = memoryOutbox{
		message:     OutboxMessage{ID: id, Topic: topic, AggregateID: aggregateID, Payload: append([]byte(nil), payload...)},
		availableAt: time.Now(),
	}
}

func (r *MemoryRepository) ClaimOutbox(_ context.Context, owner string, lease time.Duration, topics []string) (OutboxMessage, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	ids := make([]string, 0, len(r.outbox))
	for id := range r.outbox {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		item := r.outbox[id]
		if !containsTopic(topics, item.message.Topic) {
			continue
		}
		expired := item.message.LockedBy != "" && item.message.LockedAt.Add(lease).Before(now)
		if item.published || item.failed || item.availableAt.After(now) || (item.message.LockedBy != "" && !expired) {
			continue
		}
		item.message.LockedBy = owner
		item.message.LockedAt = now
		item.message.Attempts++
		r.outbox[id] = item
		return item.message, true, nil
	}
	return OutboxMessage{}, false, nil
}

func containsTopic(topics []string, candidate string) bool {
	for _, topic := range topics {
		if topic == candidate {
			return true
		}
	}
	return false
}

func (r *MemoryRepository) HeartbeatOutbox(_ context.Context, id, owner string, _ time.Duration) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.outbox[id]
	if !ok || item.published || item.failed || item.message.LockedBy != owner {
		return false, nil
	}
	item.message.LockedAt = time.Now()
	r.outbox[id] = item
	return true, nil
}

func (r *MemoryRepository) CompleteOutbox(_ context.Context, id, owner string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.outbox[id]
	if !ok || item.message.LockedBy != owner {
		return errors.New("outbox lease lost")
	}
	item.published = true
	item.message.LockedBy = ""
	r.outbox[id] = item
	return nil
}

func (r *MemoryRepository) RetryOutbox(_ context.Context, id, owner string, delay time.Duration, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.outbox[id]
	if !ok || item.message.LockedBy != owner {
		return errors.New("outbox lease lost")
	}
	item.message.LockedBy = ""
	item.message.LockedAt = time.Time{}
	item.availableAt = time.Now().Add(delay)
	r.outbox[id] = item
	return nil
}

func (r *MemoryRepository) FailOutbox(_ context.Context, id, owner, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.outbox[id]
	if !ok || item.message.LockedBy != owner {
		return errors.New("outbox lease lost")
	}
	item.failed = true
	item.message.LockedBy = ""
	r.outbox[id] = item
	return nil
}

func (r *MemoryRepository) RecordVerification(_ context.Context, evidence VerificationEvidence) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	report, ok := r.reports[evidence.RunID]
	if !ok {
		return errors.New("run report not found")
	}
	for _, assertion := range report.Assertions {
		if assertion.ID == evidence.Assertion.ID {
			return nil
		}
	}
	report.Assertions = append(report.Assertions, evidence.Assertion)
	report.Summary = fmt.Sprintf("%d of %d verification assertions passed.", passedCount(report.Assertions), len(report.Assertions))
	r.reports[evidence.RunID] = report
	return nil
}

func (r *MemoryRepository) ApplyReferenceMerchantEffect(_ context.Context, effectID string, sequence int) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	previous, exists := r.merchantEffects[effectID]
	if !exists {
		r.merchantEffects[effectID] = sequence
		return false, nil
	}
	if sequence > previous {
		r.merchantEffects[effectID] = sequence
	}
	return true, nil
}
