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
	var storedRequest []byte
	var storedResponse []byte
	err := r.pool.QueryRow(ctx, `
		SELECT request_sha256, response_body
		FROM idempotency_records
		WHERE scope = 'runs:create' AND idempotency_key_hash = $1 AND expires_at > now()
	`, keyHash[:]).Scan(&storedRequest, &storedResponse)
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
		WHERE scope = 'runs:create' AND idempotency_key_hash = $1 AND expires_at > now()
	`, keyHash[:]).Scan(&storedRequest, &storedResponse)
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
			duration_ms, event_count, finding_count, recovered, snapshot
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
	`, bundle.Run.ID, bundle.Run.ScenarioID, bundle.Run.ScenarioName, bundle.Run.Fault,
		bundle.Run.Status, bundle.Run.Score, bundle.Run.Environment, bundle.Run.StripeObjectID,
		bundle.Run.MerchantOrderID, bundle.Run.StartedAt, bundle.Run.CompletedAt,
		bundle.Run.DurationMS, bundle.Run.EventCount, bundle.Run.FindingCount, bundle.Run.Recovered, runJSON)
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
	_, err = tx.Exec(ctx, `
		INSERT INTO idempotency_records (
			scope, idempotency_key_hash, request_sha256, response_status,
			response_body, expires_at
		) VALUES ('runs:create',$1,$2,201,$3,now() + interval '24 hours')
	`, keyHash[:], requestHash[:], runJSON)
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

func (r *Repository) Events(ctx context.Context, id string) ([]domain.Event, bool, error) {
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
	var raw []byte
	err := r.pool.QueryRow(ctx, `SELECT snapshot FROM reports WHERE run_id = $1`, id).Scan(&raw)
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
	rows, err := r.pool.Query(ctx, `SELECT snapshot FROM runs ORDER BY started_at DESC`)
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
			stripe_event_id, endpoint_token_hash, event_type, livemode, body_sha256
		) VALUES ($1,$2,$3,false,$4)
		ON CONFLICT (stripe_event_id) DO NOTHING
	`, receipt.EventID, receipt.EndpointTokenSHA[:], receipt.EventType, receipt.BodySHA[:])
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
