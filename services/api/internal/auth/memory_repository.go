package auth

import (
	"context"
	"crypto/sha256"
	"sync"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/domain"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
)

type MemoryRepository struct {
	mu            sync.Mutex
	users         map[[sha256.Size]byte]IdentityRecord
	sessions      map[[sha256.Size]byte]SessionRecord
	projects      map[string]ProjectView
	environments  map[string][]Environment
	findings      map[string]map[string]domain.Finding
	notifications map[string]map[string]Notification
	connections   func(context.Context, string) ([]engine.StripeConnection, error)
}

func NewMemoryRepository(connectionLister func(context.Context, string) ([]engine.StripeConnection, error)) *MemoryRepository {
	return &MemoryRepository{users: make(map[[sha256.Size]byte]IdentityRecord), sessions: make(map[[sha256.Size]byte]SessionRecord),
		projects: make(map[string]ProjectView), environments: make(map[string][]Environment), findings: make(map[string]map[string]domain.Finding),
		notifications: make(map[string]map[string]Notification), connections: connectionLister}
}

func (r *MemoryRepository) Register(_ context.Context, input Registration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.users[input.User.EmailHash]; exists {
		return ErrAccountExists
	}
	r.users[input.User.EmailHash] = IdentityRecord{UserID: input.User.ID, EmailCiphertext: append([]byte(nil), input.User.EmailCiphertext...), PasswordHash: input.User.PasswordHash,
		OrganizationID: input.OrganizationID, OrganizationName: input.OrganizationName, Role: "owner", ProjectID: input.ProjectID, ProjectName: input.ProjectName,
		RetentionDays: input.RetentionDays}
	r.sessions[input.Session.TokenHash] = input.Session
	r.projects[input.ProjectID] = ProjectView{ID: input.ProjectID, Name: input.ProjectName, RetentionDays: input.RetentionDays}
	r.environments[input.ProjectID] = []Environment{{ID: input.ProjectID + ":local", Name: "Local", Kind: "local"}, {ID: input.ProjectID + ":sandbox", Name: "Sandbox", Kind: "sandbox", IsDefault: true}, {ID: input.ProjectID + ":staging", Name: "Staging", Kind: "staging"}}
	return nil
}

func (r *MemoryRepository) UserByEmailHash(_ context.Context, hash [sha256.Size]byte) (IdentityRecord, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.users[hash]
	return v, ok, nil
}
func (r *MemoryRepository) CreateSession(_ context.Context, session SessionRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[session.TokenHash] = session
	return nil
}
func (r *MemoryRepository) Session(_ context.Context, hash [sha256.Size]byte) (IdentityRecord, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[hash]
	if !ok {
		return IdentityRecord{}, false, nil
	}
	for _, u := range r.users {
		if u.UserID == s.UserID && u.ProjectID == s.ProjectID {
			u.SessionExpiresAt = s.ExpiresAt
			return u, true, nil
		}
	}
	return IdentityRecord{}, false, nil
}
func (r *MemoryRepository) RevokeSession(_ context.Context, hash [sha256.Size]byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, hash)
	return nil
}
func (r *MemoryRepository) Project(_ context.Context, id string) (ProjectView, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.projects[id]
	return v, ok, nil
}
func (r *MemoryRepository) UpdateProject(_ context.Context, id, name string, retention int, _ string) (ProjectView, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.projects[id]
	if !ok {
		return ProjectView{}, false, nil
	}
	if name != "" {
		v.Name = name
	}
	if retention > 0 {
		v.RetentionDays = retention
	}
	r.projects[id] = v
	for key, u := range r.users {
		if u.ProjectID == id {
			u.ProjectName = v.Name
			u.RetentionDays = v.RetentionDays
			r.users[key] = u
		}
	}
	return v, true, nil
}
func (r *MemoryRepository) Environments(_ context.Context, id string) ([]Environment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Environment(nil), r.environments[id]...), nil
}
func (r *MemoryRepository) SelectEnvironment(_ context.Context, projectID, id, _ string) (Environment, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := r.environments[projectID]
	found := false
	for _, item := range items {
		if item.ID == id {
			found = true
			break
		}
	}
	if !found {
		return Environment{}, false, nil
	}
	for i := range items {
		items[i].IsDefault = items[i].ID == id
	}
	r.environments[projectID] = items
	for _, item := range items {
		if item.ID == id {
			return item, true, nil
		}
	}
	return Environment{}, false, nil
}
func (r *MemoryRepository) Findings(_ context.Context, projectID, status string) ([]domain.Finding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := []domain.Finding{}
	for _, v := range r.findings[projectID] {
		if status == "" || (status == "resolved") == v.Resolved {
			items = append(items, v)
		}
	}
	return items, nil
}
func (r *MemoryRepository) SetFindingResolved(_ context.Context, projectID, id string, resolved bool, _ string) (domain.Finding, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.findings[projectID][id]
	if !ok {
		return domain.Finding{}, false, nil
	}
	v.Resolved = resolved
	r.findings[projectID][id] = v
	return v, true, nil
}
func (r *MemoryRepository) Notifications(_ context.Context, projectID string) ([]Notification, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	items := []Notification{}
	for _, v := range r.notifications[projectID] {
		items = append(items, v)
	}
	return items, nil
}
func (r *MemoryRepository) MarkNotificationRead(_ context.Context, projectID, id, _ string) (Notification, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.notifications[projectID][id]
	if !ok {
		return Notification{}, false, nil
	}
	now := time.Now().UTC()
	v.ReadAt = &now
	r.notifications[projectID][id] = v
	return v, true, nil
}
func (r *MemoryRepository) MarkAllNotificationsRead(_ context.Context, projectID, _ string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	count := 0
	for id, v := range r.notifications[projectID] {
		if v.ReadAt == nil {
			v.ReadAt = &now
			r.notifications[projectID][id] = v
			count++
		}
	}
	return count, nil
}
func (r *MemoryRepository) Connections(ctx context.Context, projectID string) ([]engine.StripeConnection, error) {
	if r.connections == nil {
		return []engine.StripeConnection{}, nil
	}
	return r.connections(ctx, projectID)
}
func (r *MemoryRepository) RecordAudit(context.Context, string, string, string, string, string, map[string]any) error {
	return nil
}
