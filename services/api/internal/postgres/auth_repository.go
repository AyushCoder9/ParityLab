package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/auth"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/domain"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func (r *Repository) Register(ctx context.Context, input auth.Registration) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		INSERT INTO users (id, email_hash, email_ciphertext, password_hash)
		VALUES ($1,$2,$3,$4)
	`, input.User.ID, input.User.EmailHash[:], input.User.EmailCiphertext, input.User.PasswordHash)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return auth.ErrAccountExists
		}
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO organizations (id, name) VALUES ($1,$2)`, input.OrganizationID, input.OrganizationName); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO projects (id, organization_id, name, mode, retention_days)
		VALUES ($1,$2,$3,'sandbox',$4)
	`, input.ProjectID, input.OrganizationID, input.ProjectName, input.RetentionDays); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO memberships (organization_id, user_id, role) VALUES ($1,$2,'owner')
	`, input.OrganizationID, input.User.ID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO sessions (token_hash, user_id, project_id, expires_at) VALUES ($1,$2,$3,$4)
	`, input.Session.TokenHash[:], input.Session.UserID, input.Session.ProjectID, input.Session.ExpiresAt); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `
		INSERT INTO environments (project_id, name, kind, is_default) VALUES
			($1,'Local','local',false),
			($1,'Stripe Sandbox','sandbox',true),
			($1,'Staging','staging',false)
	`, input.ProjectID); err != nil {
		return err
	}
	if err = insertAudit(ctx, tx, input.ProjectID, input.User.ID, "project.created", "project", input.ProjectID, map[string]any{"source": "registration"}); err != nil {
		return err
	}
	if err = insertAudit(ctx, tx, input.ProjectID, input.User.ID, "auth.registered", "user", input.User.ID, nil); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) UserByEmailHash(ctx context.Context, emailHash [sha256.Size]byte) (auth.IdentityRecord, bool, error) {
	return scanIdentity(r.pool.QueryRow(ctx, `
		SELECT u.id::text, u.email_ciphertext, u.password_hash,
			o.id::text, o.name, m.role, p.id::text, p.name, p.retention_days
		FROM users u
		JOIN memberships m ON m.user_id = u.id
		JOIN organizations o ON o.id = m.organization_id
		JOIN projects p ON p.organization_id = o.id
		WHERE u.email_hash = $1
		ORDER BY p.created_at
		LIMIT 1
	`, emailHash[:]), false)
}

func (r *Repository) CreateSession(ctx context.Context, session auth.SessionRecord) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO sessions (token_hash, user_id, project_id, expires_at) VALUES ($1,$2,$3,$4)
	`, session.TokenHash[:], session.UserID, session.ProjectID, session.ExpiresAt)
	return err
}

func (r *Repository) Session(ctx context.Context, tokenHash [sha256.Size]byte) (auth.IdentityRecord, bool, error) {
	return scanIdentity(r.pool.QueryRow(ctx, `
		SELECT u.id::text, u.email_ciphertext, u.password_hash,
			o.id::text, o.name, m.role, p.id::text, p.name, p.retention_days,
			s.expires_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		JOIN projects p ON p.id = s.project_id
		JOIN organizations o ON o.id = p.organization_id
		JOIN memberships m ON m.organization_id = o.id AND m.user_id = u.id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > now()
	`, tokenHash[:]), true)
}

type identityRowScanner interface{ Scan(...any) error }

func scanIdentity(row identityRowScanner, withExpiry bool) (auth.IdentityRecord, bool, error) {
	var item auth.IdentityRecord
	values := []any{
		&item.UserID, &item.EmailCiphertext, &item.PasswordHash,
		&item.OrganizationID, &item.OrganizationName, &item.Role,
		&item.ProjectID, &item.ProjectName, &item.RetentionDays,
	}
	if withExpiry {
		values = append(values, &item.SessionExpiresAt)
	}
	err := row.Scan(values...)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.IdentityRecord{}, false, nil
	}
	return item, err == nil, err
}

func (r *Repository) RevokeSession(ctx context.Context, tokenHash [sha256.Size]byte) error {
	_, err := r.pool.Exec(ctx, `UPDATE sessions SET revoked_at = COALESCE(revoked_at, now()) WHERE token_hash = $1`, tokenHash[:])
	return err
}

func (r *Repository) Project(ctx context.Context, projectID string) (auth.ProjectView, bool, error) {
	var item auth.ProjectView
	err := r.pool.QueryRow(ctx, `SELECT id::text, name, retention_days FROM projects WHERE id = $1`, projectID).Scan(&item.ID, &item.Name, &item.RetentionDays)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.ProjectView{}, false, nil
	}
	return item, err == nil, err
}

func (r *Repository) UpdateProject(ctx context.Context, projectID, name string, retentionDays int, actorID string) (auth.ProjectView, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return auth.ProjectView{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var item auth.ProjectView
	err = tx.QueryRow(ctx, `
		UPDATE projects SET
			name = CASE WHEN $2 = '' THEN name ELSE $2 END,
			retention_days = CASE WHEN $3 = 0 THEN retention_days ELSE $3 END,
			updated_at = now()
		WHERE id = $1
		RETURNING id::text, name, retention_days
	`, projectID, name, retentionDays).Scan(&item.ID, &item.Name, &item.RetentionDays)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.ProjectView{}, false, nil
	}
	if err != nil {
		return auth.ProjectView{}, false, err
	}
	if err = insertAudit(ctx, tx, projectID, actorID, "project.updated", "project", projectID, map[string]any{"name": item.Name, "retention_days": item.RetentionDays}); err != nil {
		return auth.ProjectView{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return auth.ProjectView{}, false, err
	}
	return item, true, nil
}

func (r *Repository) Environments(ctx context.Context, projectID string) ([]auth.Environment, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, name, kind, is_default FROM environments
		WHERE project_id = $1 ORDER BY kind
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]auth.Environment, 0)
	for rows.Next() {
		var item auth.Environment
		if err := rows.Scan(&item.ID, &item.Name, &item.Kind, &item.IsDefault); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) SelectEnvironment(ctx context.Context, projectID, environmentID, actorID string) (auth.Environment, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return auth.Environment{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var item auth.Environment
	err = tx.QueryRow(ctx, `
		SELECT id::text, name, kind, is_default FROM environments
		WHERE id = $1 AND project_id = $2 FOR UPDATE
	`, environmentID, projectID).Scan(&item.ID, &item.Name, &item.Kind, &item.IsDefault)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Environment{}, false, nil
	}
	if err != nil {
		return auth.Environment{}, false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE environments SET is_default = false WHERE project_id = $1 AND is_default`, projectID); err != nil {
		return auth.Environment{}, false, err
	}
	if _, err = tx.Exec(ctx, `UPDATE environments SET is_default = true WHERE project_id = $1 AND id = $2`, projectID, environmentID); err != nil {
		return auth.Environment{}, false, err
	}
	item.IsDefault = true
	if err = insertAudit(ctx, tx, projectID, actorID, "environment.selected", "environment", environmentID, map[string]any{"kind": item.Kind}); err != nil {
		return auth.Environment{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return auth.Environment{}, false, err
	}
	return item, true, nil
}

func (r *Repository) Findings(ctx context.Context, projectID, status string) ([]domain.Finding, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT f.id, f.severity, f.title, f.summary, f.cause, f.remediation, f.checkpoint, f.resolved
		FROM findings f JOIN runs r ON r.id = f.run_id
		WHERE r.project_id = $1
		  AND ($2 = '' OR ($2 = 'resolved') = f.resolved)
		ORDER BY f.created_at DESC
	`, projectID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.Finding, 0)
	for rows.Next() {
		var item domain.Finding
		if err := rows.Scan(&item.ID, &item.Severity, &item.Title, &item.Summary, &item.Cause, &item.Remediation, &item.Checkpoint, &item.Resolved); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) SetFindingResolved(ctx context.Context, projectID, findingID string, resolved bool, actorID string) (domain.Finding, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.Finding{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var item domain.Finding
	err = tx.QueryRow(ctx, `
		WITH target AS (
			SELECT f.run_id, f.id FROM findings f JOIN runs r ON r.id = f.run_id
			WHERE r.project_id = $1 AND f.id = $2 ORDER BY f.created_at DESC LIMIT 1
		)
		UPDATE findings f SET resolved = $3, resolved_at = CASE WHEN $3 THEN now() ELSE NULL END
		FROM target t WHERE f.run_id = t.run_id AND f.id = t.id
		RETURNING f.id, f.severity, f.title, f.summary, f.cause, f.remediation, f.checkpoint, f.resolved
	`, projectID, findingID, resolved).Scan(&item.ID, &item.Severity, &item.Title, &item.Summary, &item.Cause, &item.Remediation, &item.Checkpoint, &item.Resolved)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Finding{}, false, nil
	}
	if err != nil {
		return domain.Finding{}, false, err
	}
	action := "finding.reopened"
	if resolved {
		action = "finding.resolved"
	}
	if err = insertAudit(ctx, tx, projectID, actorID, action, "finding", findingID, nil); err != nil {
		return domain.Finding{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Finding{}, false, err
	}
	return item, true, nil
}

func (r *Repository) Notifications(ctx context.Context, projectID string) ([]auth.Notification, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id::text, COALESCE(run_id,''), kind, payload, read_at, created_at
		FROM notifications WHERE project_id = $1 ORDER BY created_at DESC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]auth.Notification, 0)
	for rows.Next() {
		var item auth.Notification
		var raw []byte
		if err := rows.Scan(&item.ID, &item.RunID, &item.Kind, &raw, &item.ReadAt, &item.CreatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &item.Payload); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) MarkNotificationRead(ctx context.Context, projectID, notificationID, actorID string) (auth.Notification, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return auth.Notification{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	item, ok, err := markNotificationRead(ctx, tx, projectID, notificationID)
	if err != nil || !ok {
		return item, ok, err
	}
	if err = insertAudit(ctx, tx, projectID, actorID, "notification.read", "notification", notificationID, nil); err != nil {
		return auth.Notification{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return auth.Notification{}, false, err
	}
	return item, true, nil
}

func markNotificationRead(ctx context.Context, tx pgx.Tx, projectID, notificationID string) (auth.Notification, bool, error) {
	var item auth.Notification
	var raw []byte
	err := tx.QueryRow(ctx, `
		UPDATE notifications SET read_at = COALESCE(read_at, now())
		WHERE id = $1 AND project_id = $2
		RETURNING id::text, COALESCE(run_id,''), kind, payload, read_at, created_at
	`, notificationID, projectID).Scan(&item.ID, &item.RunID, &item.Kind, &raw, &item.ReadAt, &item.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.Notification{}, false, nil
	}
	if err != nil {
		return auth.Notification{}, false, err
	}
	if err := json.Unmarshal(raw, &item.Payload); err != nil {
		return auth.Notification{}, false, err
	}
	return item, true, nil
}

func (r *Repository) MarkAllNotificationsRead(ctx context.Context, projectID, actorID string) (int, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	result, err := tx.Exec(ctx, `UPDATE notifications SET read_at = now() WHERE project_id = $1 AND read_at IS NULL`, projectID)
	if err != nil {
		return 0, err
	}
	count := int(result.RowsAffected())
	if err = insertAudit(ctx, tx, projectID, actorID, "notifications.read_all", "project", projectID, map[string]any{"updated": count}); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *Repository) Connections(ctx context.Context, projectID string) ([]engine.StripeConnection, error) {
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

func insertAudit(ctx context.Context, tx pgx.Tx, projectID, actorID, action, resourceType, resourceID string, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	var actor any
	if actorID != "" {
		actor = actorID
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_records (project_id, actor_id, action, resource_type, resource_id, metadata)
		VALUES ($1,$2,$3,$4,$5,$6)
	`, projectID, actor, action, resourceType, resourceID, raw)
	return err
}

func (r *Repository) RecordAudit(ctx context.Context, projectID, actorID, action, resourceType, resourceID string, metadata map[string]any) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := insertAudit(ctx, tx, projectID, actorID, action, resourceType, resourceID, metadata); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
