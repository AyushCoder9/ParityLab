package worker

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
	postgresadapter "github.com/ayushkumarsingh/paritylab/services/api/internal/postgres"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/secrets"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/verification"
)

type postgresWorkerStripeGateway struct{}

func (postgresWorkerStripeGateway) ValidateAccount(context.Context, string) (engine.StripeAccount, error) {
	return engine.StripeAccount{ID: "acct_worker_postgres"}, nil
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
