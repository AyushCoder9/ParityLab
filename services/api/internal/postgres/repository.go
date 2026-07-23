package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/domain"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, databaseURL, migrationsDir string) (*Repository, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	config.ConnConfig.RuntimeParams["application_name"] = "paritylab-api"
	config.MaxConns = 10
	config.MinConns = 1
	config.MaxConnLifetime = 30 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	if err := migrate(ctx, pool, migrationsDir); err != nil {
		pool.Close()
		return nil, err
	}
	return &Repository{pool: pool}, nil
}

func (r *Repository) Close() error {
	r.pool.Close()
	return nil
}

func (r *Repository) NextRunID(ctx context.Context) (string, error) {
	var number int64
	if err := r.pool.QueryRow(ctx, `SELECT nextval('run_number_seq')`).Scan(&number); err != nil {
		return "", err
	}
	return fmt.Sprintf("run_%06d", number), nil
}

func (r *Repository) ReplayRun(ctx context.Context, keyHash, requestHash [sha256.Size]byte) (domain.Run, bool, error) {
	return r.replayRun(ctx, "runs:create", keyHash, requestHash)
}

func (r *Repository) ReplayRunForProject(ctx context.Context, projectID string, keyHash, requestHash [sha256.Size]byte) (domain.Run, bool, error) {
	return r.replayRun(ctx, projectRunScope(projectID), keyHash, requestHash)
}

func (r *Repository) replayRun(ctx context.Context, scope string, keyHash, requestHash [sha256.Size]byte) (domain.Run, bool, error) {
	var storedRequest []byte
	var storedResponse []byte
	err := r.pool.QueryRow(ctx, `
		SELECT request_sha256, response_body
		FROM idempotency_records
		WHERE scope = $1 AND idempotency_key_hash = $2 AND expires_at > now()
	`, scope, keyHash[:]).Scan(&storedRequest, &storedResponse)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Run{}, false, nil
	}
	if err != nil {
		return domain.Run{}, false, err
	}
	if !bytes.Equal(storedRequest, requestHash[:]) {
		return domain.Run{}, false, engine.ErrIdempotencyConflict
	}
	var replay domain.Run
	if err := json.Unmarshal(storedResponse, &replay); err != nil {
		return domain.Run{}, false, err
	}
	return replay, true, nil
}

func (r *Repository) CreateRun(ctx context.Context, keyHash, requestHash [sha256.Size]byte, bundle engine.RunBundle) (domain.Run, bool, error) {
	return r.createRun(ctx, nil, "runs:create", keyHash, requestHash, bundle)
}

func (r *Repository) CreateRunForProject(ctx context.Context, projectID string, keyHash, requestHash [sha256.Size]byte, bundle engine.RunBundle) (domain.Run, bool, error) {
	return r.createRun(ctx, &projectID, projectRunScope(projectID), keyHash, requestHash, bundle)
}

func projectRunScope(projectID string) string {
	return "projects:" + projectID + ":runs:create"
}

func (r *Repository) createRun(ctx context.Context, projectID *string, scope string, keyHash, requestHash [sha256.Size]byte, bundle engine.RunBundle) (domain.Run, bool, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return domain.Run{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	lockID := int64(binary.BigEndian.Uint64(keyHash[:8]))
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, lockID); err != nil {
		return domain.Run{}, false, err
	}
	var storedRequest []byte
	var storedResponse []byte
	err = tx.QueryRow(ctx, `
		SELECT request_sha256, response_body
		FROM idempotency_records
		WHERE scope = $1 AND idempotency_key_hash = $2 AND expires_at > now()
	`, scope, keyHash[:]).Scan(&storedRequest, &storedResponse)
	if err == nil {
		if !bytes.Equal(storedRequest, requestHash[:]) {
			return domain.Run{}, false, engine.ErrIdempotencyConflict
		}
		var replay domain.Run
		if err := json.Unmarshal(storedResponse, &replay); err != nil {
			return domain.Run{}, false, fmt.Errorf("decode idempotency response: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.Run{}, false, err
		}
		return replay, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.Run{}, false, err
	}

	runJSON, err := json.Marshal(bundle.Run)
	if err != nil {
		return domain.Run{}, false, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO runs (
			id, scenario_id, scenario_name, fault, status, score, environment,
			stripe_object_id, merchant_order_id, started_at, completed_at,
			duration_ms, event_count, finding_count, recovered, snapshot,
			stripe_correlation_id, project_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)
	`, bundle.Run.ID, bundle.Run.ScenarioID, bundle.Run.ScenarioName, bundle.Run.Fault,
		bundle.Run.Status, bundle.Run.Score, bundle.Run.Environment, bundle.Run.StripeObjectID,
		bundle.Run.MerchantOrderID, bundle.Run.StartedAt, bundle.Run.CompletedAt,
		bundle.Run.DurationMS, bundle.Run.EventCount, bundle.Run.FindingCount, bundle.Run.Recovered,
		runJSON, nullableString(bundle.StripeCorrelationID), projectID)
	if err != nil {
		return domain.Run{}, false, fmt.Errorf("insert run: %w", err)
	}
	for _, event := range bundle.Events {
		eventJSON, marshalErr := json.Marshal(event)
		if marshalErr != nil {
			return domain.Run{}, false, marshalErr
		}
		evidenceJSON, marshalErr := json.Marshal(event.Evidence)
		if marshalErr != nil {
			return domain.Run{}, false, marshalErr
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO run_events (
				id, run_id, sequence, event_type, source, target, status, checkpoint,
				trace_id, latency_ms, evidence, occurred_at, title, detail, is_duplicate, snapshot
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		`, event.ID, event.RunID, event.Sequence, event.Type, event.Source, event.Target,
			event.Status, event.Checkpoint, event.TraceID, event.LatencyMS, evidenceJSON,
			event.At, event.Title, event.Detail, event.IsDuplicate, eventJSON)
		if err != nil {
			return domain.Run{}, false, fmt.Errorf("insert run event: %w", err)
		}
	}
	reportJSON, err := json.Marshal(bundle.Report)
	if err != nil {
		return domain.Run{}, false, err
	}
	reportHash := sha256.Sum256(reportJSON)
	_, err = tx.Exec(ctx, `
		INSERT INTO reports (run_id, snapshot, generated_at, content_sha256)
		VALUES ($1,$2,$3,$4)
	`, bundle.Run.ID, reportJSON, bundle.Report.Generated, reportHash[:])
	if err != nil {
		return domain.Run{}, false, fmt.Errorf("insert report: %w", err)
	}
	for _, assertion := range bundle.Report.Assertions {
		evidence, marshalErr := json.Marshal(map[string]string{"value": assertion.Evidence})
		if marshalErr != nil {
			return domain.Run{}, false, marshalErr
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO assertions (id, run_id, name, passed, expected, observed, evidence)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (run_id, id) DO NOTHING
		`, assertion.ID, bundle.Run.ID, assertion.Name, assertion.Passed, assertion.Expected, assertion.Observed, evidence); err != nil {
			return domain.Run{}, false, fmt.Errorf("insert assertion: %w", err)
		}
	}
	for _, finding := range bundle.Report.Findings {
		if _, err = tx.Exec(ctx, `
			INSERT INTO findings (id, run_id, severity, title, summary, checkpoint, cause, remediation, resolved)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT (run_id, id) DO NOTHING
		`, finding.ID, bundle.Run.ID, finding.Severity, finding.Title, finding.Summary, finding.Checkpoint,
			finding.Cause, finding.Remediation, finding.Resolved); err != nil {
			return domain.Run{}, false, fmt.Errorf("insert finding: %w", err)
		}
	}
	if projectID != nil {
		notificationPayload, marshalErr := json.Marshal(map[string]any{
			"run_id": bundle.Run.ID, "scenario_name": bundle.Run.ScenarioName, "status": bundle.Run.Status,
		})
		if marshalErr != nil {
			return domain.Run{}, false, marshalErr
		}
		if _, err = tx.Exec(ctx, `
			INSERT INTO notifications (project_id, run_id, kind, payload)
			VALUES ($1,$2,$3,$4)
		`, *projectID, bundle.Run.ID, "run.completed", notificationPayload); err != nil {
			return domain.Run{}, false, fmt.Errorf("insert notification: %w", err)
		}
		if err = insertAudit(ctx, tx, *projectID, "", "run.created", "run", bundle.Run.ID, map[string]any{"scenario_id": bundle.Run.ScenarioID}); err != nil {
			return domain.Run{}, false, fmt.Errorf("insert run audit: %w", err)
		}
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO idempotency_records (
			scope, idempotency_key_hash, request_sha256, response_status,
			response_body, expires_at
		) VALUES ($1,$2,$3,201,$4,now() + interval '24 hours')
	`, scope, keyHash[:], requestHash[:], runJSON)
	if err != nil {
		return domain.Run{}, false, fmt.Errorf("insert idempotency record: %w", err)
	}
	outboxPayload, err := json.Marshal(map[string]any{
		"run_id": bundle.Run.ID, "scenario_id": bundle.Run.ScenarioID,
		"status": bundle.Run.Status, "environment": "sandbox",
	})
	if err != nil {
		return domain.Run{}, false, err
	}
	topic := bundle.OutboxTopic
	if topic == "" {
		topic = "run.persisted"
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox (aggregate_type, aggregate_id, topic, payload)
		VALUES ('run',$1,$2,$3)
	`, bundle.Run.ID, topic, outboxPayload)
	if err != nil {
		return domain.Run{}, false, fmt.Errorf("insert outbox record: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Run{}, false, err
	}
	return bundle.Run, false, nil
}

func (r *Repository) Run(ctx context.Context, id string) (domain.Run, bool, error) {
	return scanRun(r.pool.QueryRow(ctx, `SELECT snapshot FROM runs WHERE id = $1`, id))
}

func (r *Repository) RunForProject(ctx context.Context, projectID, id string) (domain.Run, bool, error) {
	return scanRun(r.pool.QueryRow(ctx, `SELECT snapshot FROM runs WHERE id = $1 AND project_id = $2`, id, projectID))
}

func (r *Repository) PublicRun(ctx context.Context, id string) (domain.Run, bool, error) {
	return scanRun(r.pool.QueryRow(ctx, `SELECT snapshot FROM runs WHERE id = $1 AND project_id IS NULL`, id))
}

func (r *Repository) Events(ctx context.Context, id string) ([]domain.Event, bool, error) {
	return r.events(ctx, id, nil)
}

func (r *Repository) EventsForProject(ctx context.Context, projectID, id string) ([]domain.Event, bool, error) {
	return r.events(ctx, id, &projectID)
}

func (r *Repository) PublicEvents(ctx context.Context, id string) ([]domain.Event, bool, error) {
	var exists bool
	if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runs WHERE id = $1 AND project_id IS NULL)`, id).Scan(&exists); err != nil {
		return nil, false, err
	}
	if !exists {
		return []domain.Event{}, false, nil
	}
	return r.events(ctx, id, nil)
}

func (r *Repository) EventsAfter(ctx context.Context, id string, after, limit int) (engine.EventStreamBatch, bool, error) {
	return r.eventsAfter(ctx, id, "", false, after, limit)
}

func (r *Repository) EventsAfterForProject(ctx context.Context, projectID, id string, after, limit int) (engine.EventStreamBatch, bool, error) {
	return r.eventsAfter(ctx, id, projectID, false, after, limit)
}

func (r *Repository) PublicEventsAfter(ctx context.Context, id string, after, limit int) (engine.EventStreamBatch, bool, error) {
	return r.eventsAfter(ctx, id, "", true, after, limit)
}

func (r *Repository) eventsAfter(
	ctx context.Context,
	id string,
	projectID string,
	publicOnly bool,
	after int,
	limit int,
) (engine.EventStreamBatch, bool, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	runQuery := `SELECT snapshot FROM runs WHERE id = $1`
	args := []any{id}
	if projectID != "" {
		runQuery += ` AND project_id = $2`
		args = append(args, projectID)
	} else if publicOnly {
		runQuery += ` AND project_id IS NULL`
	}
	run, found, err := scanRun(r.pool.QueryRow(ctx, runQuery, args...))
	if err != nil || !found {
		return engine.EventStreamBatch{}, found, err
	}
	rows, err := r.pool.Query(ctx, `
		WITH selected AS (
			SELECT sequence, snapshot
			FROM run_events
			WHERE run_id = $1 AND sequence > $2
			ORDER BY sequence
			LIMIT $3
		), high_water AS (
			SELECT COALESCE(max(sequence), 0)::integer AS sequence
			FROM run_events
			WHERE run_id = $1
		)
		SELECT high_water.sequence, selected.snapshot
		FROM high_water
		LEFT JOIN selected ON true
		ORDER BY selected.sequence
	`, id, after, limit)
	if err != nil {
		return engine.EventStreamBatch{}, false, err
	}
	defer rows.Close()
	batch := engine.EventStreamBatch{Run: run, Events: make([]domain.Event, 0, limit)}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&batch.HighWater, &raw); err != nil {
			return engine.EventStreamBatch{}, false, err
		}
		if len(raw) == 0 {
			continue
		}
		var event domain.Event
		if err := json.Unmarshal(raw, &event); err != nil {
			return engine.EventStreamBatch{}, false, err
		}
		batch.Events = append(batch.Events, event)
	}
	if err := rows.Err(); err != nil {
		return engine.EventStreamBatch{}, false, err
	}
	return batch, true, nil
}

func (r *Repository) events(ctx context.Context, id string, projectID *string) ([]domain.Event, bool, error) {
	if projectID != nil {
		var exists bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runs WHERE id = $1 AND project_id = $2)`, id, *projectID).Scan(&exists); err != nil {
			return nil, false, err
		}
		if !exists {
			return []domain.Event{}, false, nil
		}
	}
	rows, err := r.pool.Query(ctx, `SELECT snapshot FROM run_events WHERE run_id = $1 ORDER BY sequence`, id)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	items := make([]domain.Event, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, false, err
		}
		var item domain.Event
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, false, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	if len(items) == 0 {
		var exists bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM runs WHERE id = $1)`, id).Scan(&exists); err != nil {
			return nil, false, err
		}
		return items, exists, nil
	}
	return items, true, nil
}

func (r *Repository) Report(ctx context.Context, id string) (domain.Report, bool, error) {
	return r.report(ctx, id, nil)
}

func (r *Repository) ReportForProject(ctx context.Context, projectID, id string) (domain.Report, bool, error) {
	return r.report(ctx, id, &projectID)
}

func (r *Repository) PublicReport(ctx context.Context, id string) (domain.Report, bool, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `
		SELECT rp.snapshot FROM reports rp JOIN runs r ON r.id = rp.run_id
		WHERE rp.run_id = $1 AND r.project_id IS NULL
	`, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Report{}, false, nil
	}
	if err != nil {
		return domain.Report{}, false, err
	}
	var report domain.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		return domain.Report{}, false, err
	}
	return report, true, nil
}

func (r *Repository) report(ctx context.Context, id string, projectID *string) (domain.Report, bool, error) {
	var raw []byte
	query := `SELECT rp.snapshot FROM reports rp JOIN runs r ON r.id = rp.run_id WHERE rp.run_id = $1`
	args := []any{id}
	if projectID != nil {
		query += ` AND r.project_id = $2`
		args = append(args, *projectID)
	}
	err := r.pool.QueryRow(ctx, query, args...).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Report{}, false, nil
	}
	if err != nil {
		return domain.Report{}, false, err
	}
	var report domain.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		return domain.Report{}, false, err
	}
	return report, true, nil
}

func (r *Repository) ListRuns(ctx context.Context) ([]domain.Run, error) {
	return r.listRuns(ctx, nil)
}

func (r *Repository) ListRunsForProject(ctx context.Context, projectID string) ([]domain.Run, error) {
	return r.listRuns(ctx, &projectID)
}

func (r *Repository) ListPublicRuns(ctx context.Context) ([]domain.Run, error) {
	rows, err := r.pool.Query(ctx, `SELECT snapshot FROM runs WHERE project_id IS NULL ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.Run, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item domain.Run
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) listRuns(ctx context.Context, projectID *string) ([]domain.Run, error) {
	query := `SELECT snapshot FROM runs`
	args := []any{}
	if projectID != nil {
		query += ` WHERE project_id = $1`
		args = append(args, *projectID)
	}
	query += ` ORDER BY started_at DESC`
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.Run, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item domain.Run
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) MarkWebhookSeen(ctx context.Context, receipt engine.WebhookReceipt) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	command, err := tx.Exec(ctx, `
		INSERT INTO webhook_events (
			stripe_event_id, endpoint_token_hash, event_type, livemode, body_sha256,
			stripe_created_at, stripe_object_id, object_status, paritylab_correlation_id
		) VALUES ($1,$2,$3,false,$4,$5,$6,$7,$8)
		ON CONFLICT (stripe_event_id) DO NOTHING
	`, receipt.EventID, receipt.EndpointTokenSHA[:], receipt.EventType, receipt.BodySHA[:],
		nullableTime(receipt.StripeCreatedAt), nullableString(receipt.StripeObjectID),
		nullableString(receipt.ObjectStatus), nullableString(receipt.CorrelationID))
	if err != nil {
		return false, err
	}
	if command.RowsAffected() == 1 {
		payload, marshalErr := json.Marshal(map[string]string{
			"stripe_event_id": receipt.EventID, "event_type": receipt.EventType,
		})
		if marshalErr != nil {
			return false, marshalErr
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO outbox (aggregate_type, aggregate_id, topic, payload)
			VALUES ('stripe_event',$1,'stripe.webhook.received',$2)
		`, receipt.EventID, payload); err != nil {
			return false, err
		}
		if err := tx.Commit(ctx); err != nil {
			return false, err
		}
		return false, nil
	}
	var storedBody []byte
	var storedEndpoint []byte
	var storedType string
	err = tx.QueryRow(ctx, `
		SELECT body_sha256, endpoint_token_hash, event_type
		FROM webhook_events WHERE stripe_event_id = $1
	`, receipt.EventID).Scan(&storedBody, &storedEndpoint, &storedType)
	if err != nil {
		return false, err
	}
	if !bytes.Equal(storedBody, receipt.BodySHA[:]) ||
		!bytes.Equal(storedEndpoint, receipt.EndpointTokenSHA[:]) || storedType != receipt.EventType {
		return false, engine.ErrWebhookConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) ProcessStripeWebhook(ctx context.Context, eventID string) (engine.WebhookProcessingResult, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return engine.WebhookProcessingResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var receipt engine.WebhookReceipt
	var processingStatus string
	var processingCode *string
	var correlatedRunID *string
	var correlatedProjectID *string
	var stripeCreatedAt *time.Time
	var stripeObjectID *string
	var objectStatus *string
	var correlationID *string
	err = tx.QueryRow(ctx, `
		SELECT event_type, processing_status, processing_error_code,
			correlated_run_id, correlated_project_id::text, stripe_created_at,
			stripe_object_id, object_status, paritylab_correlation_id
		FROM webhook_events
		WHERE stripe_event_id = $1
		FOR UPDATE
	`, eventID).Scan(
		&receipt.EventType, &processingStatus, &processingCode,
		&correlatedRunID, &correlatedProjectID, &stripeCreatedAt,
		&stripeObjectID, &objectStatus, &correlationID,
	)
	if err != nil {
		return engine.WebhookProcessingResult{}, err
	}
	receipt.EventID = eventID
	if stripeCreatedAt != nil {
		receipt.StripeCreatedAt = stripeCreatedAt.UTC()
	}
	if stripeObjectID != nil {
		receipt.StripeObjectID = *stripeObjectID
	}
	if objectStatus != nil {
		receipt.ObjectStatus = *objectStatus
	}
	if correlationID != nil {
		receipt.CorrelationID = *correlationID
	}
	if processingStatus != string(engine.WebhookPending) {
		result := engine.WebhookProcessingResult{
			EventID: eventID, EventType: receipt.EventType,
			Status: engine.WebhookProcessingStatus(processingStatus), AlreadyProcessed: true,
		}
		if processingCode != nil {
			result.ProcessingCode = *processingCode
		}
		if correlatedRunID != nil {
			result.RunID = *correlatedRunID
		}
		if correlatedProjectID != nil {
			result.ProjectID = *correlatedProjectID
		}
		if err := tx.Commit(ctx); err != nil {
			return engine.WebhookProcessingResult{}, err
		}
		return result, nil
	}

	if !engine.IsSupportedStripeWebhookType(receipt.EventType) {
		return finishWebhookProcessing(ctx, tx, receipt, engine.WebhookIgnored, "", "", "unsupported_event_type")
	}
	if receipt.StripeObjectID == "" {
		return finishWebhookProcessing(ctx, tx, receipt, engine.WebhookUnmatched, "", "", "missing_stripe_object_id")
	}
	if receipt.CorrelationID == "" {
		return finishWebhookProcessing(ctx, tx, receipt, engine.WebhookUnmatched, "", "", "missing_correlation_id")
	}

	type runCandidate struct {
		id            string
		projectID     string
		correlationID string
	}
	rows, err := tx.Query(ctx, `
		SELECT id, COALESCE(project_id::text, ''), COALESCE(stripe_correlation_id, '')
		FROM runs
		WHERE stripe_object_id = $1
		ORDER BY id
		FOR UPDATE
	`, receipt.StripeObjectID)
	if err != nil {
		return engine.WebhookProcessingResult{}, err
	}
	candidates := make([]runCandidate, 0, 2)
	for rows.Next() {
		var candidate runCandidate
		if err := rows.Scan(&candidate.id, &candidate.projectID, &candidate.correlationID); err != nil {
			rows.Close()
			return engine.WebhookProcessingResult{}, err
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return engine.WebhookProcessingResult{}, err
	}
	rows.Close()
	if len(candidates) == 0 {
		return finishWebhookProcessing(ctx, tx, receipt, engine.WebhookUnmatched, "", "", "run_not_found")
	}
	matches := make([]runCandidate, 0, 1)
	for _, candidate := range candidates {
		if candidate.correlationID == receipt.CorrelationID {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 0 {
		return finishWebhookProcessing(ctx, tx, receipt, engine.WebhookUnmatched, "", "", "correlation_mismatch")
	}
	if len(matches) != 1 {
		return finishWebhookProcessing(ctx, tx, receipt, engine.WebhookUnmatched, "", "", "ambiguous_run_match")
	}
	match := matches[0]

	var runJSON []byte
	if err := tx.QueryRow(ctx, `SELECT snapshot FROM runs WHERE id = $1 FOR UPDATE`, match.id).Scan(&runJSON); err != nil {
		return engine.WebhookProcessingResult{}, err
	}
	var run domain.Run
	if err := json.Unmarshal(runJSON, &run); err != nil {
		return engine.WebhookProcessingResult{}, err
	}
	var nextSequence int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(max(sequence), 0) + 1 FROM run_events WHERE run_id = $1
	`, match.id).Scan(&nextSequence); err != nil {
		return engine.WebhookProcessingResult{}, err
	}
	runEvent := engine.BuildWebhookRunEvent(receipt, match.id, nextSequence)
	eventJSON, err := json.Marshal(runEvent)
	if err != nil {
		return engine.WebhookProcessingResult{}, err
	}
	eventEvidenceJSON, err := json.Marshal(runEvent.Evidence)
	if err != nil {
		return engine.WebhookProcessingResult{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO run_events (
			id, run_id, sequence, event_type, source, target, status, checkpoint,
			trace_id, latency_ms, evidence, occurred_at, title, detail, is_duplicate, snapshot
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
	`, runEvent.ID, runEvent.RunID, runEvent.Sequence, runEvent.Type, runEvent.Source,
		runEvent.Target, runEvent.Status, runEvent.Checkpoint, runEvent.TraceID,
		runEvent.LatencyMS, eventEvidenceJSON, runEvent.At, runEvent.Title, runEvent.Detail,
		runEvent.IsDuplicate, eventJSON); err != nil {
		return engine.WebhookProcessingResult{}, err
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM run_events WHERE run_id = $1`, match.id).Scan(&run.EventCount); err != nil {
		return engine.WebhookProcessingResult{}, err
	}
	runJSON, err = json.Marshal(run)
	if err != nil {
		return engine.WebhookProcessingResult{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE runs SET event_count = $2, snapshot = $3 WHERE id = $1
	`, match.id, run.EventCount, runJSON); err != nil {
		return engine.WebhookProcessingResult{}, err
	}

	var reportJSON []byte
	if err := tx.QueryRow(ctx, `SELECT snapshot FROM reports WHERE run_id = $1 FOR UPDATE`, match.id).Scan(&reportJSON); err != nil {
		return engine.WebhookProcessingResult{}, err
	}
	var report domain.Report
	if err := json.Unmarshal(reportJSON, &report); err != nil {
		return engine.WebhookProcessingResult{}, err
	}
	report.Run = run
	assertion := engine.WebhookCorrelationAssertion(receipt)
	if !engine.HasAssertion(report.Assertions, assertion.ID) {
		report.Assertions = append(report.Assertions, assertion)
		report.Summary = fmt.Sprintf("%d of %d verification assertions passed.", countPassed(report.Assertions), len(report.Assertions))
		evidenceJSON, marshalErr := json.Marshal(map[string]string{
			"reference": assertion.Evidence, "checkpoint": "stripe-webhook",
		})
		if marshalErr != nil {
			return engine.WebhookProcessingResult{}, marshalErr
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO assertions (id, run_id, name, passed, expected, observed, evidence)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (run_id, id) DO NOTHING
		`, assertion.ID, match.id, assertion.Name, assertion.Passed,
			assertion.Expected, assertion.Observed, evidenceJSON); err != nil {
			return engine.WebhookProcessingResult{}, err
		}
	}
	reportJSON, err = json.Marshal(report)
	if err != nil {
		return engine.WebhookProcessingResult{}, err
	}
	reportDigest := sha256.Sum256(reportJSON)
	if _, err := tx.Exec(ctx, `
		UPDATE reports SET snapshot = $2, content_sha256 = $3 WHERE run_id = $1
	`, match.id, reportJSON, reportDigest[:]); err != nil {
		return engine.WebhookProcessingResult{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO stripe_webhook_evidence (
			stripe_event_id, run_id, project_id, event_type, stripe_object_id,
			object_status, paritylab_correlation_id, run_event_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (stripe_event_id) DO NOTHING
	`, receipt.EventID, match.id, nullableString(match.projectID), receipt.EventType,
		receipt.StripeObjectID, receipt.ObjectStatus, receipt.CorrelationID, runEvent.ID); err != nil {
		return engine.WebhookProcessingResult{}, err
	}
	return finishWebhookProcessing(ctx, tx, receipt, engine.WebhookMatched, match.id, match.projectID, "")
}

func finishWebhookProcessing(
	ctx context.Context,
	tx pgx.Tx,
	receipt engine.WebhookReceipt,
	status engine.WebhookProcessingStatus,
	runID string,
	projectID string,
	processingCode string,
) (engine.WebhookProcessingResult, error) {
	command, err := tx.Exec(ctx, `
		UPDATE webhook_events SET
			processing_status = $2,
			correlated_run_id = $3,
			correlated_project_id = $4,
			processed_at = now(),
			processing_error_code = $5
		WHERE stripe_event_id = $1 AND processing_status = 'pending'
	`, receipt.EventID, status, nullableString(runID), nullableString(projectID), nullableString(processingCode))
	if err != nil {
		return engine.WebhookProcessingResult{}, err
	}
	if command.RowsAffected() != 1 {
		return engine.WebhookProcessingResult{}, errors.New("webhook processing state changed")
	}
	if err := tx.Commit(ctx); err != nil {
		return engine.WebhookProcessingResult{}, err
	}
	return engine.WebhookProcessingResult{
		EventID: receipt.EventID, EventType: receipt.EventType, Status: status,
		RunID: runID, ProjectID: projectID, ProcessingCode: processingCode,
	}, nil
}

const (
	demoOrganizationID = "00000000-0000-7000-8000-000000000001"
	demoProjectID      = "00000000-0000-7000-8000-000000000002"
)

func (r *Repository) SaveStripeConnection(ctx context.Context, connection engine.StripeConnection) (engine.StripeConnection, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return engine.StripeConnection{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `
		INSERT INTO organizations (id, name) VALUES ($1, 'ParityLab local workspace')
		ON CONFLICT (id) DO NOTHING
	`, demoOrganizationID); err != nil {
		return engine.StripeConnection{}, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO projects (id, organization_id, name, mode)
		VALUES ($1,$2,'Default sandbox','sandbox') ON CONFLICT (id) DO NOTHING
	`, demoProjectID, demoOrganizationID); err != nil {
		return engine.StripeConnection{}, err
	}
	var stored engine.StripeConnection
	err = tx.QueryRow(ctx, `
		INSERT INTO stripe_connections (
			id, project_id, stripe_account_id, sandbox_name, secret_ciphertext,
			secret_key_version, status, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)
		ON CONFLICT (project_id, stripe_account_id) DO UPDATE SET
			sandbox_name = EXCLUDED.sandbox_name,
			secret_ciphertext = EXCLUDED.secret_ciphertext,
			secret_key_version = EXCLUDED.secret_key_version,
			status = EXCLUDED.status,
			updated_at = now()
		RETURNING id::text, stripe_account_id, sandbox_name, status, created_at,
			secret_ciphertext, secret_key_version
	`, connection.ID, demoProjectID, connection.StripeAccountID, connection.SandboxName,
		connection.SecretCiphertext, connection.SecretEncryptionKeyID, connection.Status,
		connection.CreatedAt).Scan(
		&stored.ID, &stored.StripeAccountID, &stored.SandboxName, &stored.Status,
		&stored.CreatedAt, &stored.SecretCiphertext, &stored.SecretEncryptionKeyID,
	)
	if err != nil {
		return engine.StripeConnection{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return engine.StripeConnection{}, err
	}
	return stored, nil
}

func (r *Repository) SaveStripeConnectionForProject(ctx context.Context, projectID string, connection engine.StripeConnection) (engine.StripeConnection, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return engine.StripeConnection{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var stored engine.StripeConnection
	err = tx.QueryRow(ctx, `
		INSERT INTO stripe_connections (
			id, project_id, stripe_account_id, sandbox_name, secret_ciphertext,
			secret_key_version, status, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)
		ON CONFLICT (project_id, stripe_account_id) DO UPDATE SET
			sandbox_name = EXCLUDED.sandbox_name,
			secret_ciphertext = EXCLUDED.secret_ciphertext,
			secret_key_version = EXCLUDED.secret_key_version,
			status = EXCLUDED.status,
			updated_at = now()
		RETURNING id::text, stripe_account_id, sandbox_name, status, created_at,
			secret_ciphertext, secret_key_version
	`, connection.ID, projectID, connection.StripeAccountID, connection.SandboxName,
		connection.SecretCiphertext, connection.SecretEncryptionKeyID, connection.Status,
		connection.CreatedAt).Scan(
		&stored.ID, &stored.StripeAccountID, &stored.SandboxName, &stored.Status,
		&stored.CreatedAt, &stored.SecretCiphertext, &stored.SecretEncryptionKeyID,
	)
	if err != nil {
		return engine.StripeConnection{}, err
	}
	if err := insertAudit(ctx, tx, projectID, "", "connection.validated", "stripe_connection", stored.ID, map[string]any{"stripe_account_id": stored.StripeAccountID}); err != nil {
		return engine.StripeConnection{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return engine.StripeConnection{}, err
	}
	return stored, nil
}

func (r *Repository) StripeConnection(ctx context.Context, id string) (engine.StripeConnection, bool, error) {
	var connection engine.StripeConnection
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, stripe_account_id, sandbox_name, status, created_at,
			secret_ciphertext, secret_key_version
		FROM stripe_connections WHERE id = $1
	`, id).Scan(
		&connection.ID, &connection.StripeAccountID, &connection.SandboxName,
		&connection.Status, &connection.CreatedAt, &connection.SecretCiphertext,
		&connection.SecretEncryptionKeyID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return engine.StripeConnection{}, false, nil
	}
	return connection, err == nil, err
}

func (r *Repository) StripeConnectionForProject(ctx context.Context, projectID, id string) (engine.StripeConnection, bool, error) {
	var connection engine.StripeConnection
	err := r.pool.QueryRow(ctx, `
		SELECT id::text, stripe_account_id, sandbox_name, status, created_at,
			secret_ciphertext, secret_key_version
		FROM stripe_connections WHERE id = $1 AND project_id = $2
	`, id, projectID).Scan(
		&connection.ID, &connection.StripeAccountID, &connection.SandboxName,
		&connection.Status, &connection.CreatedAt, &connection.SecretCiphertext,
		&connection.SecretEncryptionKeyID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return engine.StripeConnection{}, false, nil
	}
	return connection, err == nil, err
}

func (r *Repository) ListStripeConnectionsForProject(ctx context.Context, projectID string) ([]engine.StripeConnection, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, stripe_account_id, sandbox_name, status, created_at
		FROM stripe_connections WHERE project_id = $1 ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]engine.StripeConnection, 0)
	for rows.Next() {
		var item engine.StripeConnection
		if err := rows.Scan(&item.ID, &item.StripeAccountID, &item.SandboxName, &item.Status, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) ClaimOutbox(ctx context.Context, owner string, lease time.Duration, topics []string) (engine.OutboxMessage, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return engine.OutboxMessage{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var message engine.OutboxMessage
	var payload []byte
	err = tx.QueryRow(ctx, `
		SELECT id::text, topic, aggregate_id, payload, attempts
		FROM outbox
		WHERE published_at IS NULL AND failed_at IS NULL AND available_at <= now()
		  AND topic = ANY($2::text[])
		  AND (locked_at IS NULL OR locked_at < now() - ($1 * interval '1 millisecond'))
		ORDER BY created_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`, lease.Milliseconds(), topics).Scan(&message.ID, &message.Topic, &message.AggregateID, &payload, &message.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return engine.OutboxMessage{}, false, nil
	}
	if err != nil {
		return engine.OutboxMessage{}, false, err
	}
	message.Attempts++
	message.LockedBy = owner
	message.LockedAt = time.Now().UTC()
	message.Payload = append([]byte(nil), payload...)
	if _, err := tx.Exec(ctx, `
		UPDATE outbox SET locked_at = now(), locked_by = $2, attempts = attempts + 1
		WHERE id = $1
	`, message.ID, owner); err != nil {
		return engine.OutboxMessage{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return engine.OutboxMessage{}, false, err
	}
	return message, true, nil
}

func (r *Repository) HeartbeatOutbox(ctx context.Context, id, owner string, _ time.Duration) (bool, error) {
	command, err := r.pool.Exec(ctx, `
		UPDATE outbox SET locked_at = now()
		WHERE id = $1 AND locked_by = $2 AND published_at IS NULL AND failed_at IS NULL
	`, id, owner)
	return command.RowsAffected() == 1, err
}

func (r *Repository) CompleteOutbox(ctx context.Context, id, owner string) error {
	command, err := r.pool.Exec(ctx, `
		UPDATE outbox SET published_at = now(), locked_at = NULL, locked_by = NULL
		WHERE id = $1 AND locked_by = $2 AND published_at IS NULL AND failed_at IS NULL
	`, id, owner)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return errors.New("outbox lease lost")
	}
	return nil
}

func (r *Repository) RetryOutbox(ctx context.Context, id, owner string, delay time.Duration, errorCode string) error {
	command, err := r.pool.Exec(ctx, `
		UPDATE outbox SET available_at = now() + ($3 * interval '1 millisecond'),
			locked_at = NULL, locked_by = NULL, last_error_code = $4
		WHERE id = $1 AND locked_by = $2 AND published_at IS NULL AND failed_at IS NULL
	`, id, owner, delay.Milliseconds(), errorCode)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return errors.New("outbox lease lost")
	}
	return nil
}

func (r *Repository) FailOutbox(ctx context.Context, id, owner, errorCode string) error {
	command, err := r.pool.Exec(ctx, `
		UPDATE outbox SET failed_at = now(), locked_at = NULL, locked_by = NULL, last_error_code = $3
		WHERE id = $1 AND locked_by = $2 AND published_at IS NULL AND failed_at IS NULL
	`, id, owner, errorCode)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return errors.New("outbox lease lost")
	}
	return nil
}

func (r *Repository) RecordVerification(ctx context.Context, evidence engine.VerificationEvidence) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var raw []byte
	if err := tx.QueryRow(ctx, `SELECT snapshot FROM reports WHERE run_id = $1 FOR UPDATE`, evidence.RunID).Scan(&raw); err != nil {
		return err
	}
	var report domain.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		return err
	}
	for _, assertion := range report.Assertions {
		if assertion.ID == evidence.Assertion.ID {
			return tx.Commit(ctx)
		}
	}
	report.Assertions = append(report.Assertions, evidence.Assertion)
	report.Summary = fmt.Sprintf("%d of %d verification assertions passed.", countPassed(report.Assertions), len(report.Assertions))
	updated, err := json.Marshal(report)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(updated)
	if _, err := tx.Exec(ctx, `
		UPDATE reports SET snapshot = $2, content_sha256 = $3 WHERE run_id = $1
	`, evidence.RunID, updated, digest[:]); err != nil {
		return err
	}
	evidenceJSON, _ := json.Marshal(map[string]string{"reference": evidence.Assertion.Evidence, "checkpoint": evidence.Checkpoint})
	if _, err := tx.Exec(ctx, `
		INSERT INTO assertions (id, run_id, name, passed, expected, observed, evidence)
		VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (run_id, id) DO NOTHING
	`, evidence.Assertion.ID, evidence.RunID, evidence.Assertion.Name, evidence.Assertion.Passed,
		evidence.Assertion.Expected, evidence.Assertion.Observed, evidenceJSON); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func countPassed(assertions []domain.Assertion) int {
	count := 0
	for _, assertion := range assertions {
		if assertion.Passed {
			count++
		}
	}
	return count
}

func (r *Repository) ApplyReferenceMerchantEffect(ctx context.Context, effectID string, sequence int) (bool, error) {
	var inserted bool
	row := r.pool.QueryRow(ctx, `
		INSERT INTO reference_merchant_effects (effect_id, last_sequence)
		VALUES ($1,$2)
		ON CONFLICT (effect_id) DO UPDATE SET
			last_sequence = GREATEST(reference_merchant_effects.last_sequence, EXCLUDED.last_sequence),
			updated_at = now()
		RETURNING (xmax = 0)
	`, effectID, sequence)
	if err := row.Scan(&inserted); err != nil {
		return false, err
	}
	return !inserted, nil
}

type rowScanner interface{ Scan(...any) error }

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func scanRun(row rowScanner) (domain.Run, bool, error) {
	var raw []byte
	err := row.Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Run{}, false, nil
	}
	if err != nil {
		return domain.Run{}, false, err
	}
	var run domain.Run
	if err := json.Unmarshal(raw, &run); err != nil {
		return domain.Run{}, false, err
	}
	return run, true, nil
}
