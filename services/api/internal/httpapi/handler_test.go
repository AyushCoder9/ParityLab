package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/secrets"
)

var testNow = time.Date(2026, time.July, 18, 12, 0, 0, 0, time.UTC)

func testHandler() http.Handler {
	return New(engine.NewService(), Config{
		WebOrigin: "http://localhost:3000", WebhookSecret: "whsec_test_fixture", EndpointToken: "demo",
		Now: func() time.Time { return testNow }, SignatureMaxAge: 5 * time.Minute,
	})
}

type httpStripeGateway struct{}

func (httpStripeGateway) ValidateAccount(context.Context, string) (engine.StripeAccount, error) {
	return engine.StripeAccount{ID: "acct_test_http"}, nil
}

func (httpStripeGateway) CreatePaymentIntent(_ context.Context, _ string, input engine.StripePaymentIntentParams) (engine.StripePaymentIntent, error) {
	return engine.StripePaymentIntent{ID: "pi_test_http", Status: "requires_payment_method", Amount: input.AmountMinor, Currency: input.Currency}, nil
}

func TestStripeConnectionAndPaymentIntentRunContract(t *testing.T) {
	t.Parallel()
	repository := engine.NewMemoryRepository()
	service, err := engine.NewServiceWithRepository(repository)
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := secrets.New(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'h'}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	h := New(service, Config{
		WebOrigin: "http://localhost:3000", WebhookSecret: "whsec_test_fixture", EndpointToken: "demo",
		Stripe: engine.NewStripeService(repository, httpStripeGateway{}, cipher),
	})
	validateBody := `{"secret_key":"sk_test_never_return","sandbox_name":"QA Sandbox"}`
	validateReq := httptest.NewRequest(http.MethodPost, "/v1/connections/stripe/validate", strings.NewReader(validateBody))
	validateRec := httptest.NewRecorder()
	h.ServeHTTP(validateRec, validateReq)
	if validateRec.Code != http.StatusCreated || strings.Contains(validateRec.Body.String(), "sk_test") || strings.Contains(validateRec.Body.String(), "cipher") {
		t.Fatalf("unsafe validation response status=%d body=%s", validateRec.Code, validateRec.Body.String())
	}
	var connection engine.StripeConnection
	if err := json.Unmarshal(validateRec.Body.Bytes(), &connection); err != nil || connection.ID == "" || connection.StripeAccountID != "acct_test_http" {
		t.Fatalf("connection=%+v err=%v", connection, err)
	}
	paymentBody := fmt.Sprintf(`{"connection_id":%q,"amount_minor":1099,"currency":"usd"}`, connection.ID)
	create := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/stripe/payment-intents", strings.NewReader(paymentBody))
		req.Header.Set("Idempotency-Key", "stripe-http-idem")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	first := create()
	if first.Code != http.StatusCreated || !strings.Contains(first.Body.String(), `"stripe_object_id":"pi_test_http"`) {
		t.Fatalf("create status=%d body=%s", first.Code, first.Body.String())
	}
	replay := create()
	if replay.Code != http.StatusCreated || replay.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("replay status=%d headers=%v body=%s", replay.Code, replay.Header(), replay.Body.String())
	}
}

func TestReadEndpointsAndCORS(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path     string
		contains string
	}{
		{"/healthz", `"status":"ok"`},
		{"/v1/overview", `"readiness_score":96`},
		{"/v1/scenarios", `"checkout-duplicate"`},
		{"/v1/runs", `"object":"list"`},
		{"/v1/runs/run_000001", `"id":"run_000001"`},
		{"/v1/runs/run_000001/events", `"object":"list"`},
		{"/v1/runs/run_000001/report", `"assertions"`},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			rec := httptest.NewRecorder()
			testHandler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), tc.contains) {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
				t.Fatalf("unexpected CORS origin %q", got)
			}
			if rec.Header().Get("X-Request-ID") == "" {
				t.Fatal("missing request id")
			}
		})
	}
}

func TestCreateRunIdempotency(t *testing.T) {
	t.Parallel()
	h := testHandler()
	body := []byte(`{"scenario_id":"checkout-duplicate","fault":"duplicate"}`)
	create := func(key string, payload []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/runs", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	first := create("idem-http", body)
	if first.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", first.Code, first.Body.String())
	}
	var created map[string]any
	if err := json.Unmarshal(first.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	replay := create("idem-http", body)
	if replay.Code != http.StatusCreated || replay.Header().Get("Idempotent-Replayed") != "true" {
		t.Fatalf("replay status=%d headers=%v body=%s", replay.Code, replay.Header(), replay.Body.String())
	}
	var replayed map[string]any
	_ = json.Unmarshal(replay.Body.Bytes(), &replayed)
	if created["id"] != replayed["id"] {
		t.Fatalf("replay created a different run: %v != %v", created["id"], replayed["id"])
	}
	conflict := create("idem-http", []byte(`{"scenario_id":"checkout-duplicate","fault":"none"}`))
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), "idempotency_key_in_use") {
		t.Fatalf("expected conflict, got %d %s", conflict.Code, conflict.Body.String())
	}
	missing := create("", body)
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), "idempotency_key_missing") {
		t.Fatalf("expected missing key error, got %d %s", missing.Code, missing.Body.String())
	}
}

func TestCreateRunRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		code string
	}{
		{"malformed", `{`, "invalid_json"},
		{"missing scenario", `{"fault":"none"}`, "parameter_missing"},
		{"unknown field", `{"scenario_id":"checkout-duplicate","secret":"nope"}`, "invalid_json"},
		{"unsupported", `{"scenario_id":"checkout-duplicate","fault":"timeout"}`, "fault_not_supported"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/runs", strings.NewReader(tc.body))
			req.Header.Set("Idempotency-Key", "key-"+tc.name)
			rec := httptest.NewRecorder()
			testHandler().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tc.code) {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestEventStream(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/run_000001/events", nil).WithContext(ctx)
	req.Header.Set("Accept", "text/event-stream")
	rec := newStreamRecorder("event: run.complete")
	done := make(chan struct{})
	go func() {
		testHandler().ServeHTTP(rec, req)
		close(done)
	}()
	select {
	case <-rec.matched:
	case <-time.After(time.Second):
		t.Fatal("stream did not replay events and completion")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream did not stop after cancellation")
	}
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("status=%d content-type=%s", rec.Code, rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: run.event") || !strings.Contains(body, "event: run.complete") {
		t.Fatalf("unexpected SSE body: %s", body)
	}
	scanner := bufio.NewScanner(strings.NewReader(body))
	eventLines := 0
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "event:") {
			eventLines++
		}
	}
	if eventLines < 2 {
		t.Fatalf("expected streamed events, got %d", eventLines)
	}
}

type streamRecorder struct {
	*httptest.ResponseRecorder
	match   string
	matched chan struct{}
	once    sync.Once
}

func newStreamRecorder(match string) *streamRecorder {
	return &streamRecorder{ResponseRecorder: httptest.NewRecorder(), match: match, matched: make(chan struct{})}
}

func (r *streamRecorder) Flush() {
	r.ResponseRecorder.Flush()
	if strings.Contains(r.Body.String(), r.match) {
		r.once.Do(func() { close(r.matched) })
	}
}

func TestNotFoundUsesStripeErrorEnvelope(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/v1/runs/run_missing/report", nil)
	rec := httptest.NewRecorder()
	testHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rec.Code)
	}
	var envelope struct {
		Error struct {
			Type      string `json:"type"`
			Code      string `json:"code"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Error.Type != "invalid_request_error" || envelope.Error.Code != "run_not_found" || envelope.Error.RequestID == "" {
		t.Fatalf("unexpected error: %#v", envelope.Error)
	}
}

func TestWebhookVerificationDeduplicationAndRedaction(t *testing.T) {
	t.Parallel()
	h := testHandler()
	payload := []byte(`{"id":"evt_test_123","type":"payment_intent.succeeded","livemode":false,"data":{"object":{"billing_details":{"email":"private@example.com"},"card":"4242424242424242"}}}`)
	request := func(body []byte, signature string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/hooks/stripe/demo", bytes.NewReader(body))
		req.Header.Set("Stripe-Signature", signature)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	signature := signFixture(payload, "whsec_test_fixture", testNow)
	first := request(payload, signature)
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), `"duplicate":false`) {
		t.Fatalf("first status=%d body=%s", first.Code, first.Body.String())
	}
	if strings.Contains(first.Body.String(), "private@example.com") || strings.Contains(first.Body.String(), "424242") {
		t.Fatalf("sensitive webhook content leaked: %s", first.Body.String())
	}
	second := request(payload, signature)
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), `"duplicate":true`) {
		t.Fatalf("duplicate status=%d body=%s", second.Code, second.Body.String())
	}
	tampered := append([]byte(nil), payload...)
	tampered[len(tampered)-2] = '1'
	rejected := request(tampered, signature)
	if rejected.Code != http.StatusBadRequest || !strings.Contains(rejected.Body.String(), "signature_verification_failed") {
		t.Fatalf("tampered status=%d body=%s", rejected.Code, rejected.Body.String())
	}
}

func TestWebhookRejectsLiveAndStaleEvents(t *testing.T) {
	t.Parallel()
	h := testHandler()
	live := []byte(`{"id":"evt_live","type":"charge.succeeded","livemode":true}`)
	req := httptest.NewRequest(http.MethodPost, "/hooks/stripe/demo", bytes.NewReader(live))
	req.Header.Set("Stripe-Signature", signFixture(live, "whsec_test_fixture", testNow))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "live_mode_rejected") {
		t.Fatalf("live status=%d body=%s", rec.Code, rec.Body.String())
	}

	testBody := []byte(`{"id":"evt_stale","type":"charge.succeeded","livemode":false}`)
	staleReq := httptest.NewRequest(http.MethodPost, "/hooks/stripe/demo", bytes.NewReader(testBody))
	staleReq.Header.Set("Stripe-Signature", signFixture(testBody, "whsec_test_fixture", testNow.Add(-10*time.Minute)))
	staleRec := httptest.NewRecorder()
	h.ServeHTTP(staleRec, staleReq)
	if staleRec.Code != http.StatusBadRequest || !strings.Contains(staleRec.Body.String(), "signature_verification_failed") {
		t.Fatalf("stale status=%d body=%s", staleRec.Code, staleRec.Body.String())
	}
}

func TestPreflightAndBodyLimit(t *testing.T) {
	t.Parallel()
	preflight := httptest.NewRequest(http.MethodOptions, "/v1/runs", nil)
	preflightRec := httptest.NewRecorder()
	testHandler().ServeHTTP(preflightRec, preflight)
	if preflightRec.Code != http.StatusNoContent {
		t.Fatalf("preflight status=%d", preflightRec.Code)
	}

	tooLarge := io.LimitReader(strings.NewReader(strings.Repeat("a", maxRequestBody+2)), maxRequestBody+2)
	req := httptest.NewRequest(http.MethodPost, "/v1/runs", tooLarge)
	req.Header.Set("Idempotency-Key", "large")
	rec := httptest.NewRecorder()
	testHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "request_too_large") {
		t.Fatalf("large status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func signFixture(payload []byte, secret string, at time.Time) string {
	timestamp := strconv.FormatInt(at.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "." + string(payload)))
	return fmt.Sprintf("t=%s,v1=%s", timestamp, hex.EncodeToString(mac.Sum(nil)))
}
