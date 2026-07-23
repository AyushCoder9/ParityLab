package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/auth"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/domain"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/secrets"
)

func TestEventStreamResumesAfterStableSequence(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/v1/runs/run_000001/events", nil).WithContext(ctx)
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Last-Event-ID", "3")
	response := newStreamRecorder("id: 4\n")
	done := make(chan struct{})
	go func() {
		testHandler().ServeHTTP(response, request)
		close(done)
	}()
	select {
	case <-response.matched:
	case <-time.After(time.Second):
		t.Fatal("resumed stream did not deliver sequence 4")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("resumed stream did not stop after cancellation")
	}
	body := response.Body.String()
	if !strings.Contains(body, "retry: 2000\n\n") || !strings.Contains(body, "event: run.complete") {
		t.Fatalf("missing retry or completion framing: %s", body)
	}
	for _, skipped := range []string{"id: 1\n", "id: 2\n", "id: 3\n"} {
		if strings.Contains(body, skipped) {
			t.Fatalf("resume replayed acknowledged event %q: %s", skipped, body)
		}
	}
}

func TestLastEventIDIsStrictAndCannotLeadHighWater(t *testing.T) {
	t.Parallel()
	for _, values := range [][]string{
		{""}, {"01"}, {"+1"}, {"-1"}, {" 1"}, {"1 "}, {"1.0"}, {"2147483648"}, {"1", "2"},
	} {
		if _, apiErr := parseLastEventID(values); apiErr == nil || apiErr.Code != "invalid_last_event_id" {
			t.Fatalf("accepted cursor values=%q error=%+v", values, apiErr)
		}
	}
	for raw, expected := range map[string]int{"0": 0, "1": 1, "2147483647": 2147483647} {
		got, apiErr := parseLastEventID([]string{raw})
		if apiErr != nil || got != expected {
			t.Fatalf("cursor=%q got=%d error=%+v", raw, got, apiErr)
		}
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/runs/run_000001/events", nil)
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Last-Event-ID", "999")
	response := httptest.NewRecorder()
	testHandler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"last_event_id_ahead"`) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestEventStreamEmitsHeartbeatAndHonorsCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/v1/runs/run_000001/events", nil).WithContext(ctx)
	request.Header.Set("Accept", "text/event-stream")
	response := newStreamRecorder(": heartbeat\n\n")
	handler := New(engine.NewService(), Config{
		WebOrigin: "http://localhost:3000", SSEPollInterval: 20 * time.Millisecond,
		SSEHeartbeat: 5 * time.Millisecond, SSERetry: 7 * time.Millisecond,
	})
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(response, request)
		close(done)
	}()
	select {
	case <-response.matched:
	case <-time.After(time.Second):
		t.Fatal("stream did not emit heartbeat")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stream leaked after request cancellation")
	}
	if !strings.Contains(response.Body.String(), "retry: 7\n\n") {
		t.Fatalf("custom retry hint missing: %s", response.Body.String())
	}
}

func TestEventStreamDeliversEventAppendedAfterCompletion(t *testing.T) {
	t.Parallel()
	repository := engine.NewMemoryRepository()
	service, err := engine.NewServiceWithRepository(repository)
	if err != nil {
		t.Fatal(err)
	}
	cipher, err := secrets.New(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'s'}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	stripeService := engine.NewStripeService(repository, httpStripeGateway{}, cipher)
	connection, apiErr := stripeService.ValidateConnection(context.Background(), "sk_test_sse", "SSE Sandbox")
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	idempotencyKey := "sse-appended-event"
	run, _, apiErr := stripeService.CreatePaymentIntentRun(
		context.Background(), connection.ID, 1099, "usd", idempotencyKey, []byte(`{"amount_minor":1099}`),
	)
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	handler := New(service, Config{
		WebOrigin: "http://localhost:3000", SSEPollInterval: 5 * time.Millisecond,
		SSEHeartbeat: 50 * time.Millisecond, SSERetry: 10 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/v1/runs/"+run.ID+"/events", nil).WithContext(ctx)
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Last-Event-ID", fmt.Sprintf("%d", run.EventCount))
	response := newAppendStreamRecorder("event: run.complete", `"stripe_event_id":"evt_sse_append"`)
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(response, request)
		close(done)
	}()
	select {
	case <-response.completed:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("stream did not emit truthful run completion")
	}

	correlationHash := sha256.Sum256([]byte("paritylab:stripe:" + idempotencyKey))
	correlationID := "plcorr_" + hex.EncodeToString(correlationHash[:12])
	webhookBody := []byte(fmt.Sprintf(
		`{"created":1770000000,"data":{"object":{"id":"%s","status":"succeeded","metadata":{"paritylab_correlation_id":"%s","private":"do-not-stream"}}}}`,
		run.StripeObjectID, correlationID,
	))
	if duplicate, err := service.RecordWebhook(
		"evt_sse_append", "payment_intent.succeeded", "demo", webhookBody,
	); err != nil || duplicate {
		cancel()
		t.Fatalf("record webhook duplicate=%v err=%v", duplicate, err)
	}
	if result, err := repository.ProcessStripeWebhook(context.Background(), "evt_sse_append"); err != nil || result.Status != engine.WebhookMatched {
		cancel()
		t.Fatalf("process result=%+v err=%v", result, err)
	}
	select {
	case <-response.appended:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("live stream did not deliver appended database event")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("appended-event stream did not stop")
	}
	body := response.Body.String()
	if strings.Contains(body, "do-not-stream") ||
		!strings.Contains(body, fmt.Sprintf("id: %d\n", run.EventCount+1)) {
		t.Fatalf("unsafe or unstable appended event: %s", body)
	}
}

func TestEventStreamPreservesPublicAndTenantIsolation(t *testing.T) {
	t.Parallel()
	cipher, err := secrets.New(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'i'}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	repository := engine.NewMemoryRepository()
	service, err := engine.NewServiceWithRepository(repository)
	if err != nil {
		t.Fatal(err)
	}
	authRepository := auth.NewMemoryRepository(repository.ListStripeConnectionsForProject)
	handler := New(service, Config{
		WebOrigin: "http://127.0.0.1:3202", Auth: auth.NewService(authRepository, cipher),
		Stripe: engine.NewStripeService(repository, httpStripeGateway{}, cipher), InsecureCookies: true,
		SSEPollInterval: 5 * time.Millisecond, SSEHeartbeat: 20 * time.Millisecond,
	})
	register := httptest.NewRequest(http.MethodPost, "/v1/auth/register", strings.NewReader(
		`{"email":"sse-owner@example.test","password":"correct-horse-battery","workspace_name":"Workspace","project_name":"Project"}`,
	))
	register.Header.Set("Content-Type", "application/json")
	register.Header.Set("Origin", "http://127.0.0.1:3202")
	registered := httptest.NewRecorder()
	handler.ServeHTTP(registered, register)
	var identity auth.SessionView
	if registered.Code != http.StatusCreated || json.Unmarshal(registered.Body.Bytes(), &identity) != nil {
		t.Fatalf("register status=%d body=%s", registered.Code, registered.Body.String())
	}
	cookies := registered.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("registration cookies=%v", cookies)
	}
	tenantRun, _, createErr := service.CreateRunForProject(
		identity.Project.ID, "checkout-duplicate", domain.FaultDuplicate, "sse-tenant-run", []byte(`{}`),
	)
	if createErr != nil {
		t.Fatal(createErr)
	}

	anonymousPrivate := httptest.NewRequest(http.MethodGet, "/v1/runs/"+tenantRun.ID+"/events", nil)
	anonymousPrivate.Header.Set("Accept", "text/event-stream")
	anonymousResponse := httptest.NewRecorder()
	handler.ServeHTTP(anonymousResponse, anonymousPrivate)
	if anonymousResponse.Code != http.StatusNotFound {
		t.Fatalf("anonymous tenant access status=%d body=%s", anonymousResponse.Code, anonymousResponse.Body.String())
	}
	authenticatedPublic := httptest.NewRequest(http.MethodGet, "/v1/runs/run_000001/events", nil)
	authenticatedPublic.Header.Set("Accept", "text/event-stream")
	authenticatedPublic.AddCookie(cookies[0])
	publicResponse := httptest.NewRecorder()
	handler.ServeHTTP(publicResponse, authenticatedPublic)
	if publicResponse.Code != http.StatusNotFound {
		t.Fatalf("tenant crossed public boundary status=%d body=%s", publicResponse.Code, publicResponse.Body.String())
	}

	ctx, cancel := context.WithCancel(context.Background())
	tenantRequest := httptest.NewRequest(http.MethodGet, "/v1/runs/"+tenantRun.ID+"/events", nil).WithContext(ctx)
	tenantRequest.Header.Set("Accept", "text/event-stream")
	tenantRequest.AddCookie(cookies[0])
	tenantResponse := newStreamRecorder("event: run.complete")
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(tenantResponse, tenantRequest)
		close(done)
	}()
	select {
	case <-tenantResponse.matched:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("tenant stream did not open")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("tenant stream did not cancel")
	}
}

func TestSSEPreflightAllowsResumeHeader(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest(http.MethodOptions, "/v1/runs/run_000001/events", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	request.Header.Set("Access-Control-Request-Headers", "Last-Event-ID")
	response := httptest.NewRecorder()
	testHandler().ServeHTTP(response, request)
	if response.Code != http.StatusNoContent ||
		!strings.Contains(response.Header().Get("Access-Control-Allow-Headers"), "Last-Event-ID") {
		t.Fatalf("status=%d headers=%v", response.Code, response.Header())
	}
}

func TestSSERefreshesBoundedWriteDeadline(t *testing.T) {
	t.Parallel()
	writer := &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	before := time.Now()
	if err := refreshSSEWriteDeadline(writer, 250*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if !writer.deadline.After(before) || writer.deadline.After(before.Add(time.Second)) {
		t.Fatalf("unexpected SSE write deadline: %s", writer.deadline)
	}
}

type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadline time.Time
}

func (r *deadlineRecorder) SetWriteDeadline(deadline time.Time) error {
	r.deadline = deadline
	return nil
}

type appendStreamRecorder struct {
	*httptest.ResponseRecorder
	completeMatch string
	appendMatch   string
	completed     chan struct{}
	appended      chan struct{}
	completeOnce  sync.Once
	appendOnce    sync.Once
}

func newAppendStreamRecorder(completeMatch, appendMatch string) *appendStreamRecorder {
	return &appendStreamRecorder{
		ResponseRecorder: httptest.NewRecorder(), completeMatch: completeMatch, appendMatch: appendMatch,
		completed: make(chan struct{}), appended: make(chan struct{}),
	}
}

func (r *appendStreamRecorder) Flush() {
	r.ResponseRecorder.Flush()
	body := r.Body.String()
	if strings.Contains(body, r.completeMatch) {
		r.completeOnce.Do(func() { close(r.completed) })
	}
	if strings.Contains(body, r.appendMatch) {
		r.appendOnce.Do(func() { close(r.appended) })
	}
}
