package worker

import (
	"context"
	"encoding/base64"
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

func TestWorkerDoesNotAcknowledgeUnhandledWebhookTopic(t *testing.T) {
	t.Parallel()
	repository := engine.NewMemoryRepository()
	service, err := engine.NewServiceWithRepository(repository)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate, err := service.RecordWebhook("evt_unhandled", "payment_intent.succeeded", "demo", []byte(`{"livemode":false}`)); err != nil || duplicate {
		t.Fatalf("webhook duplicate=%v err=%v", duplicate, err)
	}
	signer, _ := verification.NewSigner("unhandled-topic-signing-secret")
	instance := New(repository, verification.NewRelay(signer, NewRepositoryMerchant(repository, signer)), Config{ID: "verification-only"})
	if processed, err := instance.RunOnce(context.Background()); err != nil || processed {
		t.Fatalf("verification worker consumed unsupported topic processed=%v err=%v", processed, err)
	}
	message, ok, err := repository.ClaimOutbox(context.Background(), "webhook-future-worker", time.Minute, []string{"stripe.webhook.received"})
	if err != nil || !ok || message.AggregateID != "evt_unhandled" {
		t.Fatalf("webhook job was lost message=%+v ok=%v err=%v", message, ok, err)
	}
}
