package postgres

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/auth"
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

func TestPostgresEventsAfterUsesStableCursorAndTenantBoundary(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	repository, err := Open(ctx, databaseURL, "../../../../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()
	service, err := engine.NewServiceWithRepository(repository)
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%012x", time.Now().UnixNano()&0xffffffffffff)
	userID := "12000000-0000-4000-8000-" + suffix
	projectID := "32000000-0000-4000-8000-" + suffix
	if err := repository.Register(ctx, auth.Registration{
		User: auth.User{
			ID: userID, EmailHash: sha256.Sum256([]byte("sse-" + suffix)),
			EmailCiphertext: []byte("ciphertext"), PasswordHash: "password-hash",
		},
		OrganizationID: "22000000-0000-4000-8000-" + suffix, OrganizationName: "SSE Integration",
		ProjectID: projectID, ProjectName: "SSE Integration", RetentionDays: 30,
		Session: auth.SessionRecord{
			TokenHash: sha256.Sum256([]byte("sse-session-" + suffix)),
			UserID:    userID, ProjectID: projectID, ExpiresAt: time.Now().Add(time.Hour),
		},
	}); err != nil {
		t.Fatal(err)
	}
	run, _, apiErr := service.CreateRunForProject(
		projectID, "checkout-duplicate", domain.FaultDuplicate, "sse-pg-"+suffix, []byte(`{}`),
	)
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	batch, found, err := repository.EventsAfterForProject(ctx, projectID, run.ID, 3, 2)
	if err != nil || !found || len(batch.Events) != 2 ||
		batch.Events[0].Sequence != 4 || batch.Events[1].Sequence != 5 ||
		batch.HighWater != run.EventCount {
		t.Fatalf("batch=%+v found=%v err=%v", batch, found, err)
	}
	resumed, found, err := repository.EventsAfterForProject(ctx, projectID, run.ID, 5, 100)
	if err != nil || !found || len(resumed.Events) == 0 || resumed.Events[0].Sequence != 6 {
		t.Fatalf("resumed=%+v found=%v err=%v", resumed, found, err)
	}
	if _, found, err := repository.EventsAfterForProject(
		ctx, "42000000-0000-4000-8000-"+suffix, run.ID, 0, 100,
	); err != nil || found {
		t.Fatalf("cross-tenant found=%v err=%v", found, err)
	}
	if _, found, err := repository.PublicEventsAfter(ctx, run.ID, 0, 100); err != nil || found {
		t.Fatalf("private run crossed public boundary found=%v err=%v", found, err)
	}
}
