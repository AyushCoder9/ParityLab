package worker

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/secrets"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/verification"
)

type workerStripeGateway struct{}

func (workerStripeGateway) ValidateAccount(context.Context, string) (engine.StripeAccount, error) {
	return engine.StripeAccount{ID: "acct_worker"}, nil
}

func (workerStripeGateway) CreatePaymentIntent(_ context.Context, _ string, input engine.StripePaymentIntentParams) (engine.StripePaymentIntent, error) {
	return engine.StripePaymentIntent{ID: "pi_worker", Status: "succeeded", Amount: input.AmountMinor, Currency: input.Currency}, nil
}

func TestQueuedStripeRunIsClaimedAndVerifiedExactlyOnce(t *testing.T) {
	t.Parallel()
	repository := engine.NewMemoryRepository()
	cipher, _ := secrets.New(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("w", 32))))
	stripeService := engine.NewStripeService(repository, workerStripeGateway{}, cipher)
	connection, apiErr := stripeService.ValidateConnection(context.Background(), "sk_test_worker", "Worker sandbox")
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	body := []byte(`{"amount_minor":1099,"currency":"usd"}`)
	run, _, apiErr := stripeService.CreatePaymentIntentRun(context.Background(), connection.ID, 1099, "usd", "worker-idem", body)
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	signer, _ := verification.NewSigner("worker-reference-signing-secret")
	w := New(repository, verification.NewRelay(signer, NewRepositoryMerchant(repository, signer)), Config{ID: "worker-a"})
	processed := false
	for range 8 {
		ok, err := w.RunOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			break
		}
		processed = true
	}
	if !processed {
		t.Fatal("worker did not claim outbox")
	}
	report, ok, err := repository.Report(context.Background(), run.ID)
	if err != nil || !ok {
		t.Fatalf("report missing ok=%v err=%v", ok, err)
	}
	found := false
	for _, assertion := range report.Assertions {
		if assertion.ID == "assert_reference_merchant_exactly_once" && assertion.Passed {
			found = true
		}
	}
	if !found {
		t.Fatalf("worker assertion missing: %+v", report.Assertions)
	}
}

func TestExpiredLeaseCanBeRecoveredByAnotherWorker(t *testing.T) {
	t.Parallel()
	repository := engine.NewMemoryRepository()
	if _, err := engine.NewServiceWithRepository(repository); err != nil {
		t.Fatal(err)
	}
	first, ok, err := repository.ClaimOutbox(context.Background(), "worker-one", time.Millisecond, []string{"run.persisted"})
	if err != nil || !ok {
		t.Fatalf("first claim=%+v ok=%v err=%v", first, ok, err)
	}
	time.Sleep(3 * time.Millisecond)
	second, ok, err := repository.ClaimOutbox(context.Background(), "worker-two", time.Millisecond, []string{"run.persisted"})
	if err != nil || !ok || second.ID != first.ID || second.Attempts != first.Attempts+1 {
		t.Fatalf("recovery first=%+v second=%+v ok=%v err=%v", first, second, ok, err)
	}
}

func TestWorkerDoesNotAcknowledgeUnhandledOutboxTopic(t *testing.T) {
	t.Parallel()
	repository := engine.NewMemoryRepository()
	service, err := engine.NewServiceWithRepository(repository)
	if err != nil {
		t.Fatal(err)
	}
	_, _, apiErr := service.CreateRun("checkout-duplicate", "duplicate", "unhandled-topic", []byte(`{}`))
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	signer, _ := verification.NewSigner("unhandled-topic-signing-secret")
	instance := New(repository, verification.NewRelay(signer, NewRepositoryMerchant(repository, signer)), Config{ID: "verification-only"})
	if processed, err := instance.RunOnce(context.Background()); err != nil || processed {
		t.Fatalf("verification worker consumed unsupported topic processed=%v err=%v", processed, err)
	}
	message, ok, err := repository.ClaimOutbox(context.Background(), "future-worker", time.Minute, []string{"run.persisted"})
	if err != nil || !ok || message.Topic != "run.persisted" {
		t.Fatalf("unknown job was lost message=%+v ok=%v err=%v", message, ok, err)
	}
}

func TestWebhookConsumerCorrelatesOnceAndProjectsSanitizedEvidence(t *testing.T) {
	t.Parallel()
	repository := engine.NewMemoryRepository()
	cipher, _ := secrets.New(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("c", 32))))
	stripeService := engine.NewStripeService(repository, workerStripeGateway{}, cipher)
	connection, apiErr := stripeService.ValidateConnection(context.Background(), "sk_test_webhook", "Webhook sandbox")
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	idempotencyKey := "webhook-correlation"
	run, _, apiErr := stripeService.CreatePaymentIntentRun(
		context.Background(), connection.ID, 1099, "usd", idempotencyKey, []byte(`{"amount_minor":1099}`),
	)
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	correlationHash := sha256.Sum256([]byte("paritylab:stripe:" + idempotencyKey))
	correlationID := "plcorr_" + hex.EncodeToString(correlationHash[:12])
	service, err := engine.NewServiceWithRepository(repository)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(fmt.Sprintf(
		`{"created":1770000000,"data":{"object":{"id":"%s","status":"succeeded","metadata":{"paritylab_correlation_id":"%s","private":"must-not-persist"}}}}`,
		run.StripeObjectID, correlationID,
	))
	if duplicate, err := service.RecordWebhook("evt_webhook_match", "payment_intent.succeeded", "demo", body); err != nil || duplicate {
		t.Fatalf("record webhook duplicate=%v err=%v", duplicate, err)
	}
	signer, _ := verification.NewSigner("webhook-correlation-signing-secret")
	instance := New(repository, verification.NewRelay(signer, NewRepositoryMerchant(repository, signer)), Config{ID: "webhook-worker"})
	for range 4 {
		processed, err := instance.RunOnce(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if !processed {
			break
		}
	}
	events, ok, err := repository.Events(context.Background(), run.ID)
	if err != nil || !ok {
		t.Fatalf("events ok=%v err=%v", ok, err)
	}
	webhookEvents := 0
	for _, event := range events {
		if event.Checkpoint != "stripe-webhook" {
			continue
		}
		webhookEvents++
		encoded := fmt.Sprintf("%v", event.Evidence)
		if strings.Contains(encoded, "must-not-persist") || event.Evidence["stripe_event_id"] != "evt_webhook_match" {
			t.Fatalf("unsafe or incomplete evidence: %+v", event.Evidence)
		}
	}
	if webhookEvents != 1 {
		t.Fatalf("webhook event count=%d events=%+v", webhookEvents, events)
	}
	report, ok, err := repository.Report(context.Background(), run.ID)
	if err != nil || !ok {
		t.Fatalf("report ok=%v err=%v", ok, err)
	}
	assertions := 0
	for _, assertion := range report.Assertions {
		if assertion.ID == "assert_stripe_webhook_correlated" && assertion.Passed {
			assertions++
		}
	}
	if assertions != 1 {
		t.Fatalf("correlation assertion count=%d report=%+v", assertions, report.Assertions)
	}
	result, err := repository.ProcessStripeWebhook(context.Background(), "evt_webhook_match")
	if err != nil || !result.AlreadyProcessed || result.Status != engine.WebhookMatched {
		t.Fatalf("replay result=%+v err=%v", result, err)
	}
	eventsAfterReplay, _, _ := repository.Events(context.Background(), run.ID)
	if len(eventsAfterReplay) != len(events) {
		t.Fatalf("replay duplicated evidence before=%d after=%d", len(events), len(eventsAfterReplay))
	}
}

func TestWebhookConsumerDurablyUnmatchesMissingCorrelationAndIgnoresUnknownType(t *testing.T) {
	t.Parallel()
	repository := engine.NewMemoryRepository()
	service, err := engine.NewServiceWithRepository(repository)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate, err := service.RecordWebhook(
		"evt_missing_correlation", "payment_intent.succeeded", "demo",
		[]byte(`{"data":{"object":{"id":"pi_untrusted","status":"succeeded"}}}`),
	); err != nil || duplicate {
		t.Fatalf("record missing correlation duplicate=%v err=%v", duplicate, err)
	}
	if duplicate, err := service.RecordWebhook(
		"evt_future_type", "payment_intent.future_state", "demo",
		[]byte(`{"data":{"object":{"id":"pi_untrusted","metadata":{"paritylab_correlation_id":"plcorr_untrusted"}}}}`),
	); err != nil || duplicate {
		t.Fatalf("record future type duplicate=%v err=%v", duplicate, err)
	}
	signer, _ := verification.NewSigner("webhook-terminal-signing-secret")
	instance := New(repository, verification.NewRelay(signer, NewRepositoryMerchant(repository, signer)), Config{ID: "webhook-terminal"})
	for range 2 {
		if processed, err := instance.RunOnce(context.Background()); err != nil || !processed {
			t.Fatalf("processed=%v err=%v", processed, err)
		}
	}
	unmatched, err := repository.ProcessStripeWebhook(context.Background(), "evt_missing_correlation")
	if err != nil || unmatched.Status != engine.WebhookUnmatched ||
		unmatched.ProcessingCode != "missing_correlation_id" || !unmatched.AlreadyProcessed {
		t.Fatalf("unmatched result=%+v err=%v", unmatched, err)
	}
	ignored, err := repository.ProcessStripeWebhook(context.Background(), "evt_future_type")
	if err != nil || ignored.Status != engine.WebhookIgnored ||
		ignored.ProcessingCode != "unsupported_event_type" || !ignored.AlreadyProcessed {
		t.Fatalf("ignored result=%+v err=%v", ignored, err)
	}
}
