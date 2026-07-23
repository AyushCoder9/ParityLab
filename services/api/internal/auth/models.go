package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/domain"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
)

var ErrAccountExists = errors.New("account already exists")

type User struct {
	ID              string
	EmailCiphertext []byte
	EmailHash       [sha256.Size]byte
	PasswordHash    string
}

type SessionRecord struct {
	TokenHash [sha256.Size]byte
	UserID    string
	ProjectID string
	ExpiresAt time.Time
}

type Registration struct {
	User             User
	OrganizationID   string
	OrganizationName string
	ProjectID        string
	ProjectName      string
	RetentionDays    int
	Session          SessionRecord
}

type IdentityRecord struct {
	UserID           string
	EmailCiphertext  []byte
	PasswordHash     string
	OrganizationID   string
	OrganizationName string
	Role             string
	ProjectID        string
	ProjectName      string
	RetentionDays    int
	SessionExpiresAt time.Time
}

type UserView struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type OrganizationView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type ProjectView struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	RetentionDays int    `json:"retention_days"`
}

type SessionView struct {
	Authenticated bool             `json:"authenticated"`
	User          UserView         `json:"user"`
	Organization  OrganizationView `json:"organization"`
	Project       ProjectView      `json:"project"`
	ExpiresAt     time.Time        `json:"expires_at"`
}

type Environment struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	IsDefault bool   `json:"is_default"`
}

type Notification struct {
	ID        string         `json:"id"`
	RunID     string         `json:"run_id,omitempty"`
	Kind      string         `json:"kind"`
	Payload   map[string]any `json:"payload"`
	ReadAt    *time.Time     `json:"read_at,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

type Repository interface {
	Register(context.Context, Registration) error
	UserByEmailHash(context.Context, [sha256.Size]byte) (IdentityRecord, bool, error)
	CreateSession(context.Context, SessionRecord) error
	Session(context.Context, [sha256.Size]byte) (IdentityRecord, bool, error)
	RevokeSession(context.Context, [sha256.Size]byte) error
	Project(context.Context, string) (ProjectView, bool, error)
	UpdateProject(context.Context, string, string, int, string) (ProjectView, bool, error)
	Environments(context.Context, string) ([]Environment, error)
	SelectEnvironment(context.Context, string, string, string) (Environment, bool, error)
	Findings(context.Context, string, string) ([]domain.Finding, error)
	SetFindingResolved(context.Context, string, string, bool, string) (domain.Finding, bool, error)
	Notifications(context.Context, string) ([]Notification, error)
	MarkNotificationRead(context.Context, string, string, string) (Notification, bool, error)
	MarkAllNotificationsRead(context.Context, string, string) (int, error)
	Connections(context.Context, string) ([]engine.StripeConnection, error)
	RecordAudit(context.Context, string, string, string, string, string, map[string]any) error
}
