package stripeadapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
)

func TestOfficialSDKAdapterValidatesAndCreatesPaymentIntent(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk_test_fixture" {
			t.Fatalf("unexpected authorization header %q", got)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/account":
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "acct_test_paritylab", "object": "account"})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/payment_intents":
			body := readForm(t, r)
			if body.Get("amount") != "1099" || body.Get("currency") != "usd" ||
				body.Get("metadata[paritylab_correlation_id]") != "plcorr_test" {
				t.Fatalf("unexpected form: %v", body)
			}
			if r.Header.Get("Idempotency-Key") != "pl_test" {
				t.Fatalf("unexpected idempotency key %q", r.Header.Get("Idempotency-Key"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "pi_test_paritylab", "object": "payment_intent", "status": "requires_payment_method",
				"amount": 1099, "currency": "usd",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := New(server.URL, server.Client())
	account, err := client.ValidateAccount(context.Background(), "sk_test_fixture")
	if err != nil || account.ID != "acct_test_paritylab" {
		t.Fatalf("account=%+v err=%v", account, err)
	}
	intent, err := client.CreatePaymentIntent(context.Background(), "sk_test_fixture", engine.StripePaymentIntentParams{
		AmountMinor: 1099, Currency: "usd", IdempotencyKey: "pl_test",
		Metadata: map[string]string{"paritylab_correlation_id": "plcorr_test"},
	})
	if err != nil || intent.ID != "pi_test_paritylab" || intent.Amount != 1099 || intent.Currency != "usd" {
		t.Fatalf("intent=%+v err=%v", intent, err)
	}
}

func TestOfficialSDKAdapterRejectsLiveKeyBeforeNetwork(t *testing.T) {
	t.Parallel()
	client := New("http://127.0.0.1:1", nil)
	if _, err := client.ValidateAccount(context.Background(), "sk_live_forbidden"); err == nil {
		t.Fatal("live key reached adapter")
	}
}

func readForm(t *testing.T, r *http.Request) url.Values {
	t.Helper()
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/x-www-form-urlencoded") {
		t.Fatalf("unexpected content type %q", r.Header.Get("Content-Type"))
	}
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}
	return r.Form
}
