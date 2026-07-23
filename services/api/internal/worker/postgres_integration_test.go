package worker

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/auth"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
	postgresadapter "github.com/ayushkumarsingh/paritylab/services/api/internal/postgres"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/secrets"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/verification"
)

type postgresWorkerStripeGateway struct{}

func (postgresWorkerStripeGateway) ValidateAccount(context.Context, string) (engine.StripeAccount, error) {
	return engine.StripeAccount{ID: "acct_worker_postgres"}, nil
}

func TestPostgresWebhookConsumerCorrelatesTenantRunAndRejectsMissingCorrelation(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	repository, err := postgresadapter.Open(ctx, databaseURL, "../../../../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	suffix := fmt.Sprintf("%012x", time.Now().UnixNano()&0xffffffffffff)
	projectID := "31000000-0000-4000-8000-" + suffix
	registration := auth.Registration{
		User: auth.User{
			ID: "11000000-0000-4000-8000-" + suffix, EmailHash: sha256.Sum256([]byte("webhook-" + suffix)),
			EmailCiphertext: []byte("ciphertext"), PasswordHash: "password-hash",
		},
		OrganizationID: "21000000-0000-4000-8000-" + suffix, OrganizationName: "Webhook Integration",
		ProjectID: projectID, ProjectName: "Webhook Integration", RetentionDays: 30,
		Session: auth.SessionRecord{
			TokenHash: sha256.Sum256([]byte("webhook-session-" + suffix)),
			UserID:    "11000000-0000-4000-8000-" + suffix,
			ProjectID: projectID, ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	if err := repository.Register(ctx, registration); err != nil {
		t.Fatal(err)
	}
	cipher, _ := secrets.New(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("t", 32))))
	stripeService := engine.NewStripeService(repository, postgresWorkerStripeGateway{}, cipher)
	connection, apiErr := stripeService.ValidateConnectionForProject(
		ctx, projectID, "sk_test_worker_webhook_postgres", "Tenant webhook PG",
	)
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	idempotencyKey := "tenant-webhook-" + suffix
	run, _, apiErr := stripeService.CreatePaymentIntentRunForProject(
		ctx, projectID, connection.ID, 2099, "usd", idempotencyKey, []byte(idempotencyKey),
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
	eventID := "evt_tenant_" + suffix
	body := []byte(fmt.Sprintf(
		`{"created":1770000000,"data":{"object":{"id":"%s","status":"succeeded","metadata":{"paritylab_correlation_id":"%s"}}}}`,
		run.StripeObjectID, correlationID,
	))
	if duplicate, err := service.RecordWebhook(eventID, "payment_intent.succeeded", "demo", body); err != nil || duplicate {
		t.Fatalf("record webhook duplicate=%v err=%v", duplicate, err)
	}
	signer, _ := verification.NewSigner("postgres-webhook-signing-secret")
	instance := New(repository, verification.NewRelay(signer, NewRepositoryMerchant(repository, signer)), Config{
		ID: "webhook-pg-" + suffix, Lease: time.Second,
	})
	for range 8 {
		processed, err := instance.RunOnce(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !processed {
			break
		}
	}
	result, err := repository.ProcessStripeWebhook(ctx, eventID)
	if err != nil || !result.AlreadyProcessed || result.Status != engine.WebhookMatched ||
		result.RunID != run.ID || result.ProjectID != projectID {
		t.Fatalf("processing result=%+v err=%v", result, err)
	}
	events, ok, err := repository.EventsForProject(ctx, projectID, run.ID)
	if err != nil || !ok {
		t.Fatalf("events ok=%v err=%v", ok, err)
	}
	projected := 0
	for _, event := range events {
		if event.Checkpoint == "stripe-webhook" {
			projected++
		}
	}
	if projected != 1 {
		t.Fatalf("projected webhook events=%d", projected)
	}
	report, ok, err := repository.ReportForProject(ctx, projectID, run.ID)
	if err != nil || !ok || !engine.HasAssertion(report.Assertions, "assert_stripe_webhook_correlated") {
		t.Fatalf("report ok=%v err=%v assertions=%+v", ok, err, report.Assertions)
	}

	missingID := "evt_missing_" + suffix
	if duplicate, err := service.RecordWebhook(
		missingID, "payment_intent.succeeded", "demo",
		[]byte(fmt.Sprintf(`{"data":{"object":{"id":"%s","status":"succeeded"}}}`, run.StripeObjectID)),
	); err != nil || duplicate {
		t.Fatalf("record missing correlation duplicate=%v err=%v", duplicate, err)
	}
	if processed, err := instance.RunOnce(ctx); err != nil || !processed {
		t.Fatalf("process missing correlation=%v err=%v", processed, err)
	}
	unmatched, err := repository.ProcessStripeWebhook(ctx, missingID)
	if err != nil || unmatched.Status != engine.WebhookUnmatched ||
		unmatched.ProcessingCode != "missing_correlation_id" || unmatched.ProjectID != "" {
		t.Fatalf("unmatched result=%+v err=%v", unmatched, err)
	}
}

func (postgresWorkerStripeGateway) CreatePaymentIntent(_ context.Context, _ string, input engine.StripePaymentIntentParams) (engine.StripePaymentIntent, error) {
	return engine.StripePaymentIntent{ID: "pi_worker_postgres", Status: "succeeded", Amount: input.AmountMinor, Currency: input.Currency}, nil
}

func TestPostgresWorkerClaimsQueuedRunAndPersistsAssertion(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	repository, err := postgresadapter.Open(ctx, databaseURL, "../../../../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	if _, err := engine.NewServiceWithRepository(repository); err != nil {
		t.Fatal(err)
	}
	cipher, _ := secrets.New(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("p", 32))))
	stripeService := engine.NewStripeService(repository, postgresWorkerStripeGateway{}, cipher)
	connection, apiErr := stripeService.ValidateConnection(ctx, "sk_test_worker_postgres", "Worker PG")
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	idempotencyKey := fmt.Sprintf("worker-pg-%d", time.Now().UnixNano())
	run, _, apiErr := stripeService.CreatePaymentIntentRun(ctx, connection.ID, 1099, "usd", idempotencyKey, []byte(idempotencyKey))
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	signer, _ := verification.NewSigner("postgres-worker-signing-secret")
	instance := New(repository, verification.NewRelay(signer, NewRepositoryMerchant(repository, signer)), Config{ID: "worker-pg", Lease: time.Second})
	for range 32 {
		processed, err := instance.RunOnce(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !processed {
			break
		}
	}
	report, ok, err := repository.Report(ctx, run.ID)
	if err != nil || !ok {
		t.Fatalf("report ok=%v err=%v", ok, err)
	}
	for _, assertion := range report.Assertions {
		if assertion.ID == "assert_reference_merchant_exactly_once" && assertion.Passed {
			return
		}
	}
	t.Fatalf("durable worker assertion missing: %+v", report.Assertions)
}
