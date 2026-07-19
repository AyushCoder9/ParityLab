package engine

import (
	"context"
	"encoding/base64"
	"strings"
	"sync"
	"testing"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/secrets"
)

type recordingStripeGateway struct {
	mu     sync.Mutex
	inputs []StripePaymentIntentParams
}

func (g *recordingStripeGateway) ValidateAccount(context.Context, string) (StripeAccount, error) {
	return StripeAccount{ID: "acct_test_engine"}, nil
}

func (g *recordingStripeGateway) CreatePaymentIntent(_ context.Context, _ string, input StripePaymentIntentParams) (StripePaymentIntent, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inputs = append(g.inputs, input)
	return StripePaymentIntent{ID: "pi_test_engine", Status: "requires_payment_method", Amount: input.AmountMinor, Currency: input.Currency}, nil
}

func TestStripePaymentIntentReplayUsesIdenticalStripeParameters(t *testing.T) {
	t.Parallel()
	repository := NewMemoryRepository()
	cipher, err := secrets.New(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("e", 32))))
	if err != nil {
		t.Fatal(err)
	}
	gateway := &recordingStripeGateway{}
	service := NewStripeService(repository, gateway, cipher)
	connection, apiErr := service.ValidateConnection(context.Background(), "sk_test_fixture", "Test sandbox")
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	body := []byte(`{"connection_id":"` + connection.ID + `","amount_minor":1099,"currency":"usd"}`)
	first, replayed, apiErr := service.CreatePaymentIntentRun(context.Background(), connection.ID, 1099, "usd", "client-idem", body)
	if apiErr != nil || replayed {
		t.Fatalf("first replayed=%v err=%v", replayed, apiErr)
	}
	second, replayed, apiErr := service.CreatePaymentIntentRun(context.Background(), connection.ID, 1099, "usd", "client-idem", body)
	if apiErr != nil || !replayed || second.ID != first.ID {
		t.Fatalf("replay run=%+v replayed=%v err=%v", second, replayed, apiErr)
	}
	gateway.mu.Lock()
	callCount := len(gateway.inputs)
	gateway.mu.Unlock()
	if callCount != 1 {
		t.Fatalf("Stripe retry parameters changed: %#v", gateway.inputs)
	}
	if _, _, conflict := service.CreatePaymentIntentRun(context.Background(), connection.ID, 2099, "usd", "client-idem", []byte(`{"different":true}`)); conflict == nil || conflict.HTTPStatus != 409 {
		t.Fatalf("expected local idempotency conflict, got %#v", conflict)
	}
	gateway.mu.Lock()
	callCount = len(gateway.inputs)
	gateway.mu.Unlock()
	if callCount != 1 {
		t.Fatalf("idempotency conflict reached Stripe: %#v", gateway.inputs)
	}
	report, ok, err := repository.Report(context.Background(), first.ID)
	if err != nil || !ok || report.Run.StripeObjectID != "pi_test_engine" || report.Run.ID != first.ID {
		t.Fatalf("persisted report missing real intent evidence: ok=%v report=%+v err=%v", ok, report, err)
	}
}

func TestValidateSandboxSecret(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"sk_live_x", "rk_live_x", "pk_live_x", "pk_test_x", "garbage"} {
		if ValidateSandboxSecret(key) == nil {
			t.Fatalf("accepted invalid key %q", key)
		}
	}
	for _, key := range []string{"sk_test_x", "rk_test_x"} {
		if err := ValidateSandboxSecret(key); err != nil {
			t.Fatalf("rejected sandbox key %q: %v", key, err)
		}
	}
}

func TestStripePaymentIntentWithoutEncryptionConfigurationReturns503(t *testing.T) {
	t.Parallel()
	service := NewStripeService(NewMemoryRepository(), &recordingStripeGateway{}, nil)
	_, _, apiErr := service.CreatePaymentIntentRun(context.Background(), "00000000-0000-4000-8000-000000000001", 1099, "usd", "idem", []byte(`{}`))
	if apiErr == nil || apiErr.HTTPStatus != 503 || apiErr.Code != "connection_storage_not_configured" {
		t.Fatalf("expected safe 503, got %#v", apiErr)
	}
}
