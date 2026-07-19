package engine

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/domain"
)

func TestSeededServiceProvidesDemoOverview(t *testing.T) {
	t.Parallel()
	s := NewService()
	overview := s.Overview()
	if overview.ReadinessScore != 96 || overview.Environment != "sandbox" {
		t.Fatalf("unexpected overview: %#v", overview)
	}
	if overview.Stats.TotalRuns != 3 || len(overview.RecentRuns) != 3 {
		t.Fatalf("expected three seeded runs, got stats=%+v recent=%d", overview.Stats, len(overview.RecentRuns))
	}
	if len(s.Scenarios()) != 5 {
		t.Fatalf("expected five scenarios, got %d", len(s.Scenarios()))
	}
}

func TestWebhookEventIDCannotBeReusedWithDifferentContent(t *testing.T) {
	t.Parallel()
	s := NewService()
	if duplicate, err := s.RecordWebhook("evt_same", "charge.succeeded", "demo", []byte(`{"amount":1000,"currency":"usd"}`)); err != nil || duplicate {
		t.Fatalf("first receipt duplicate=%v err=%v", duplicate, err)
	}
	if duplicate, err := s.RecordWebhook("evt_same", "charge.succeeded", "demo", []byte(`{"amount":2000,"currency":"usd"}`)); duplicate || !errors.Is(err, ErrWebhookConflict) {
		t.Fatalf("changed receipt duplicate=%v err=%v", duplicate, err)
	}
}

func TestFirstWebhookPersistsAndEnqueuesExactlyOnce(t *testing.T) {
	t.Parallel()
	repository := NewMemoryRepository()
	service, err := NewServiceWithRepository(repository)
	if err != nil {
		t.Fatal(err)
	}
	// Drain deterministic seed messages before checking webhook ingress.
	for range 3 {
		message, ok, err := repository.ClaimOutbox(context.Background(), "drain", time.Minute, []string{"run.persisted"})
		if err != nil || !ok {
			t.Fatalf("drain ok=%v err=%v", ok, err)
		}
		if err := repository.CompleteOutbox(context.Background(), message.ID, "drain"); err != nil {
			t.Fatal(err)
		}
	}
	if duplicate, err := service.RecordWebhook("evt_queue", "payment_intent.succeeded", "demo", []byte(`{"livemode":false}`)); err != nil || duplicate {
		t.Fatalf("first duplicate=%v err=%v", duplicate, err)
	}
	if duplicate, err := service.RecordWebhook("evt_queue", "payment_intent.succeeded", "demo", []byte(`{"livemode":false}`)); err != nil || !duplicate {
		t.Fatalf("replay duplicate=%v err=%v", duplicate, err)
	}
	message, ok, err := repository.ClaimOutbox(context.Background(), "webhook-worker", time.Minute, []string{"stripe.webhook.received"})
	if err != nil || !ok || message.Topic != "stripe.webhook.received" || message.AggregateID != "evt_queue" {
		t.Fatalf("message=%+v ok=%v err=%v", message, ok, err)
	}
	if err := repository.CompleteOutbox(context.Background(), message.ID, "webhook-worker"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := repository.ClaimOutbox(context.Background(), "webhook-worker", time.Minute, []string{"stripe.webhook.received"}); err != nil || ok {
		t.Fatalf("duplicate webhook enqueued another job ok=%v err=%v", ok, err)
	}
}

func TestCreateRunAndIdempotentReplay(t *testing.T) {
	t.Parallel()
	s := NewService()
	body := []byte(`{"scenario_id":"checkout-duplicate","fault":"duplicate"}`)
	created, replayed, apiErr := s.CreateRun("checkout-duplicate", domain.FaultDuplicate, "idem-1", body)
	if apiErr != nil || replayed {
		t.Fatalf("first create failed: replayed=%v error=%v", replayed, apiErr)
	}
	replay, replayed, apiErr := s.CreateRun("checkout-duplicate", domain.FaultDuplicate, "idem-1", body)
	if apiErr != nil || !replayed || replay.ID != created.ID {
		t.Fatalf("replay mismatch: created=%s replay=%s replayed=%v error=%v", created.ID, replay.ID, replayed, apiErr)
	}
	_, _, apiErr = s.CreateRun("checkout-duplicate", domain.FaultNone, "idem-1", []byte(`{"scenario_id":"checkout-duplicate","fault":"none"}`))
	if apiErr == nil || apiErr.HTTPStatus != 409 || apiErr.Code != "idempotency_key_in_use" {
		t.Fatalf("expected idempotency conflict, got %#v", apiErr)
	}
}

func TestCreateRunValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		scenarioID string
		fault      domain.Fault
		key        string
		wantCode   string
	}{
		{"missing key", "checkout-duplicate", domain.FaultNone, "", "idempotency_key_missing"},
		{"missing scenario", "not-real", domain.FaultNone, "key-a", "scenario_not_found"},
		{"unsupported fault", "checkout-duplicate", domain.FaultTimeout, "key-b", "fault_not_supported"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, apiErr := NewService().CreateRun(tc.scenarioID, tc.fault, tc.key, []byte(tc.name))
			if apiErr == nil || apiErr.Code != tc.wantCode {
				t.Fatalf("expected %s, got %#v", tc.wantCode, apiErr)
			}
		})
	}
}

func TestFaultTimelinesConvergeOrBlock(t *testing.T) {
	t.Parallel()
	tests := []struct {
		scenario string
		fault    domain.Fault
		status   domain.RunStatus
		balanced bool
		marker   domain.EventStatus
	}{
		{"checkout-duplicate", domain.FaultDuplicate, domain.RunPassed, true, domain.EventDiverged},
		{"webhook-disorder", domain.FaultReorder, domain.RunPassed, true, domain.EventDiverged},
		{"endpoint-recovery", domain.FaultTimeout, domain.RunPassed, true, domain.EventDiverged},
		{"webhook-disorder", domain.FaultTamper, domain.RunFailed, false, domain.EventBlocked},
	}
	for i, tc := range tests {
		body := []byte(tc.scenario + string(tc.fault))
		run, _, apiErr := NewService().CreateRun(tc.scenario, tc.fault, "fault-key-"+string(rune('a'+i)), body)
		if apiErr != nil {
			t.Fatalf("%s: %v", tc.fault, apiErr)
		}
		if run.Status != tc.status {
			t.Fatalf("%s: expected status %s, got %s", tc.fault, tc.status, run.Status)
		}
		events, ok := NewService().Events("missing")
		if ok || events != nil {
			t.Fatal("missing run unexpectedly returned events")
		}
		events, _ = func() ([]domain.Event, bool) {
			s := NewService()
			r, _, _ := s.CreateRun(tc.scenario, tc.fault, "timeline-"+string(rune('a'+i)), body)
			return s.Events(r.ID)
		}()
		found := false
		for _, event := range events {
			if event.Status == tc.marker {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s: expected timeline marker %s", tc.fault, tc.marker)
		}
		s := NewService()
		r, _, _ := s.CreateRun(tc.scenario, tc.fault, "report-"+string(rune('a'+i)), body)
		report, _ := s.Report(r.ID)
		if report.State.Balanced != tc.balanced {
			t.Fatalf("%s: expected balanced=%v", tc.fault, tc.balanced)
		}
	}
}

func TestWebhookDeduplicationIsConcurrentSafe(t *testing.T) {
	t.Parallel()
	s := NewService()
	const callers = 32
	var wg sync.WaitGroup
	results := make(chan bool, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- s.MarkWebhookSeen("evt_shared")
		}()
	}
	wg.Wait()
	close(results)
	firstDeliveries := 0
	for duplicate := range results {
		if !duplicate {
			firstDeliveries++
		}
	}
	if firstDeliveries != 1 {
		t.Fatalf("expected one first delivery, got %d", firstDeliveries)
	}
}
