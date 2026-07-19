package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/domain"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
)

func TestRepositoryPersistsAcrossRestart(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	testSuffix := time.Now().UnixNano()
	idempotencyKey := fmt.Sprintf("integration-idempotency-%d", testSuffix)
	webhookID := fmt.Sprintf("evt_integration_restart_%d", testSuffix)
	firstRepo, err := Open(ctx, databaseURL, "../../../../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	first, err := engine.NewServiceWithRepository(firstRepo)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"scenario_id":"checkout-duplicate","fault":"duplicate"}`)
	run, replayed, apiErr := first.CreateRun("checkout-duplicate", domain.FaultDuplicate, idempotencyKey, body)
	if apiErr != nil || replayed {
		t.Fatalf("create replayed=%v error=%v", replayed, apiErr)
	}
	if duplicate, err := first.RecordWebhook(webhookID, "payment_intent.succeeded", "demo", []byte(`{"livemode":false}`)); err != nil || duplicate {
		t.Fatalf("first webhook duplicate=%v error=%v", duplicate, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	secondRepo, err := Open(ctx, databaseURL, "../../../../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	second, err := engine.NewServiceWithRepository(secondRepo)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	stored, ok := second.Run(run.ID)
	if !ok || stored.ID != run.ID {
		t.Fatalf("run missing after restart: ok=%v run=%+v", ok, stored)
	}
	replayedRun, replayed, apiErr := second.CreateRun("checkout-duplicate", domain.FaultDuplicate, idempotencyKey, body)
	if apiErr != nil || !replayed || replayedRun.ID != run.ID {
		t.Fatalf("idempotency lost after restart: replayed=%v run=%+v error=%v", replayed, replayedRun, apiErr)
	}
	if duplicate, err := second.RecordWebhook(webhookID, "payment_intent.succeeded", "demo", []byte(`{"livemode":false}`)); err != nil || !duplicate {
		t.Fatalf("webhook dedup lost after restart: duplicate=%v error=%v", duplicate, err)
	}
	if events, ok := second.Events(run.ID); !ok || len(events) == 0 {
		t.Fatalf("events missing after restart: ok=%v count=%d", ok, len(events))
	}
	if report, ok := second.Report(run.ID); !ok || report.Run.ID != run.ID {
		t.Fatalf("report missing after restart: ok=%v report=%+v", ok, report)
	}
}
