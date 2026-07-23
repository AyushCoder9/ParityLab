package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/domain"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/secrets"
	"golang.org/x/crypto/argon2"
)

const SessionTTL = 24 * time.Hour

type Service struct {
	repo   Repository
	cipher *secrets.Cipher
	now    func() time.Time
}

func NewService(repo Repository, cipher *secrets.Cipher) *Service {
	return &Service{repo: repo, cipher: cipher, now: time.Now}
}

func (s *Service) Register(ctx context.Context, email, password, organizationName, projectName string) (SessionView, string, *domain.Error) {
	if s == nil || s.repo == nil || s.cipher == nil {
		return SessionView{}, "", unavailable()
	}
	normalized, validation := validateCredentials(email, password)
	if validation != nil {
		return SessionView{}, "", validation
	}
	if strings.TrimSpace(organizationName) == "" || strings.TrimSpace(projectName) == "" {
		return SessionView{}, "", domain.Invalid("parameter_missing", "workspace_name and project_name are required.", "workspace_name")
	}
	userID, err := randomUUID()
	if err != nil {
		return SessionView{}, "", domain.Internal("identity_creation_failed", "The account could not be created.")
	}
	organizationID, err := randomUUID()
	if err != nil {
		return SessionView{}, "", domain.Internal("identity_creation_failed", "The account could not be created.")
	}
	projectID, err := randomUUID()
	if err != nil {
		return SessionView{}, "", domain.Internal("identity_creation_failed", "The account could not be created.")
	}
	passwordHash, err := hashPassword(password)
	if err != nil {
		return SessionView{}, "", domain.Internal("password_hash_failed", "The account could not be created.")
	}
	emailCiphertext, err := s.cipher.Encrypt([]byte(normalized), userID)
	if err != nil {
		return SessionView{}, "", domain.Internal("email_encryption_failed", "The account could not be created.")
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return SessionView{}, "", domain.Internal("session_creation_failed", "The session could not be created.")
	}
	expires := s.now().UTC().Add(SessionTTL)
	registration := Registration{
		User:           User{ID: userID, EmailCiphertext: emailCiphertext, EmailHash: s.cipher.BlindIndex(normalized), PasswordHash: passwordHash},
		OrganizationID: organizationID, OrganizationName: strings.TrimSpace(organizationName),
		ProjectID: projectID, ProjectName: strings.TrimSpace(projectName), RetentionDays: 30,
		Session: SessionRecord{TokenHash: tokenHash, UserID: userID, ProjectID: projectID, ExpiresAt: expires},
	}
	if err := s.repo.Register(ctx, registration); err != nil {
		if errors.Is(err, ErrAccountExists) {
			return SessionView{}, "", domain.Conflict("account_exists", "An account with this email already exists.", "email")
		}
		return SessionView{}, "", domain.Internal("registration_persistence_failed", "The account could not be stored.")
	}
	return sessionView(IdentityRecord{
		UserID: userID, EmailCiphertext: emailCiphertext, OrganizationID: organizationID,
		OrganizationName: registration.OrganizationName, Role: "owner", ProjectID: projectID,
		ProjectName: registration.ProjectName, RetentionDays: 30, SessionExpiresAt: expires,
	}, normalized), token, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (SessionView, string, *domain.Error) {
	if s == nil || s.repo == nil || s.cipher == nil {
		return SessionView{}, "", unavailable()
	}
	normalized := strings.ToLower(strings.TrimSpace(email))
	identity, ok, lookupErr := s.repo.UserByEmailHash(ctx, s.cipher.BlindIndex(normalized))
	passwordHash := dummyPasswordHash
	if ok && lookupErr == nil {
		passwordHash = identity.PasswordHash
	}
	passwordValid := verifyPassword(passwordHash, password)
	if lookupErr != nil || !ok || !passwordValid {
		return SessionView{}, "", domain.Unauthorized("invalid_credentials", "The email or password is incorrect.")
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return SessionView{}, "", domain.Internal("session_creation_failed", "The session could not be created.")
	}
	identity.SessionExpiresAt = s.now().UTC().Add(SessionTTL)
	if err := s.repo.CreateSession(ctx, SessionRecord{TokenHash: tokenHash, UserID: identity.UserID, ProjectID: identity.ProjectID, ExpiresAt: identity.SessionExpiresAt}); err != nil {
		return SessionView{}, "", domain.Internal("session_persistence_failed", "The session could not be stored.")
	}
	if err := s.repo.RecordAudit(ctx, identity.ProjectID, identity.UserID, "auth.login", "session", "current", nil); err != nil {
		_ = s.repo.RevokeSession(ctx, tokenHash)
		return SessionView{}, "", domain.Internal("audit_persistence_failed", "The session could not be stored.")
	}
	decrypted, err := s.cipher.Decrypt(identity.EmailCiphertext, identity.UserID)
	if err != nil {
		return SessionView{}, "", domain.Internal("identity_decryption_failed", "The account could not be loaded.")
	}
	defer clear(decrypted)
	return sessionView(identity, string(decrypted)), token, nil
}

var dummyPasswordHash = func() string {
	salt := []byte("paritylab-dummy!")
	hash := argon2.IDKey([]byte("not-a-user-password"), salt, 1, 64*1024, 4, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=1,p=4$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash))
}()

func (s *Service) Session(ctx context.Context, token string) (SessionView, *domain.Error) {
	if s == nil || s.repo == nil || s.cipher == nil || token == "" {
		return SessionView{}, domain.Unauthorized("session_required", "Authentication is required.")
	}
	hash := sha256.Sum256([]byte(token))
	identity, ok, err := s.repo.Session(ctx, hash)
	if err != nil || !ok || !identity.SessionExpiresAt.After(s.now().UTC()) {
		return SessionView{}, domain.Unauthorized("session_invalid", "The session is invalid or expired.")
	}
	decrypted, err := s.cipher.Decrypt(identity.EmailCiphertext, identity.UserID)
	if err != nil {
		return SessionView{}, domain.Unauthorized("session_invalid", "The session is invalid or expired.")
	}
	defer clear(decrypted)
	return sessionView(identity, string(decrypted)), nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if s == nil || s.repo == nil || token == "" {
		return nil
	}
	hash := sha256.Sum256([]byte(token))
	identity, ok, _ := s.repo.Session(ctx, hash)
	if err := s.repo.RevokeSession(ctx, hash); err != nil {
		return err
	}
	if ok {
		return s.repo.RecordAudit(ctx, identity.ProjectID, identity.UserID, "auth.logout", "session", "current", nil)
	}
	return nil
}

func (s *Service) LoginThrottleKey(email string) string {
	if s == nil || s.cipher == nil {
		return ""
	}
	normalized := strings.ToLower(strings.TrimSpace(email))
	index := s.cipher.BlindIndex(normalized)
	return base64.RawURLEncoding.EncodeToString(index[:])
}

func (s *Service) Project(ctx context.Context, projectID string) (ProjectView, *domain.Error) {
	item, ok, err := s.repo.Project(ctx, projectID)
	if err != nil {
		return ProjectView{}, domain.Internal("project_lookup_failed", "The project could not be loaded.")
	}
	if !ok {
		return ProjectView{}, domain.NotFound("project", projectID)
	}
	return item, nil
}

func (s *Service) UpdateProject(ctx context.Context, identity SessionView, name string, retentionDays int) (ProjectView, *domain.Error) {
	name = strings.TrimSpace(name)
	if name == "" && retentionDays == 0 {
		return ProjectView{}, domain.Invalid("parameter_missing", "name or retention_days is required.", "")
	}
	if len(name) > 120 {
		return ProjectView{}, domain.Invalid("invalid_project_name", "name must be at most 120 characters.", "name")
	}
	if retentionDays != 0 && (retentionDays < 1 || retentionDays > 3650) {
		return ProjectView{}, domain.Invalid("invalid_retention_days", "retention_days must be between 1 and 3650.", "retention_days")
	}
	item, ok, err := s.repo.UpdateProject(ctx, identity.Project.ID, name, retentionDays, identity.User.ID)
	if err != nil {
		return ProjectView{}, domain.Internal("project_update_failed", "The project could not be updated.")
	}
	if !ok {
		return ProjectView{}, domain.NotFound("project", identity.Project.ID)
	}
	return item, nil
}

func (s *Service) Environments(ctx context.Context, projectID string) ([]Environment, *domain.Error) {
	items, err := s.repo.Environments(ctx, projectID)
	if err != nil {
		return nil, domain.Internal("environment_lookup_failed", "Environments could not be loaded.")
	}
	return items, nil
}

func (s *Service) SelectEnvironment(ctx context.Context, identity SessionView, id string) (Environment, *domain.Error) {
	item, ok, err := s.repo.SelectEnvironment(ctx, identity.Project.ID, id, identity.User.ID)
	if err != nil {
		return Environment{}, domain.Internal("environment_update_failed", "The environment could not be selected.")
	}
	if !ok {
		return Environment{}, domain.NotFound("environment", id)
	}
	return item, nil
}

func (s *Service) Findings(ctx context.Context, projectID, status string) ([]domain.Finding, *domain.Error) {
	if status == "all" {
		status = ""
	}
	if status != "" && status != "open" && status != "resolved" {
		return nil, domain.Invalid("invalid_status", "status must be open, resolved, or all.", "status")
	}
	items, err := s.repo.Findings(ctx, projectID, status)
	if err != nil {
		return nil, domain.Internal("finding_lookup_failed", "Findings could not be loaded.")
	}
	return items, nil
}

func (s *Service) SetFindingResolved(ctx context.Context, identity SessionView, id string, resolved bool) (domain.Finding, *domain.Error) {
	item, ok, err := s.repo.SetFindingResolved(ctx, identity.Project.ID, id, resolved, identity.User.ID)
	if err != nil {
		return domain.Finding{}, domain.Internal("finding_update_failed", "The finding could not be updated.")
	}
	if !ok {
		return domain.Finding{}, domain.NotFound("finding", id)
	}
	return item, nil
}

func (s *Service) Notifications(ctx context.Context, projectID string) ([]Notification, *domain.Error) {
	items, err := s.repo.Notifications(ctx, projectID)
	if err != nil {
		return nil, domain.Internal("notification_lookup_failed", "Notifications could not be loaded.")
	}
	return items, nil
}

func (s *Service) MarkNotificationRead(ctx context.Context, identity SessionView, id string) (Notification, *domain.Error) {
	item, ok, err := s.repo.MarkNotificationRead(ctx, identity.Project.ID, id, identity.User.ID)
	if err != nil {
		return Notification{}, domain.Internal("notification_update_failed", "The notification could not be updated.")
	}
	if !ok {
		return Notification{}, domain.NotFound("notification", id)
	}
	return item, nil
}

func (s *Service) MarkAllNotificationsRead(ctx context.Context, identity SessionView) (int, *domain.Error) {
	count, err := s.repo.MarkAllNotificationsRead(ctx, identity.Project.ID, identity.User.ID)
	if err != nil {
		return 0, domain.Internal("notification_update_failed", "Notifications could not be updated.")
	}
	return count, nil
}

func (s *Service) Connections(ctx context.Context, projectID string) ([]engine.StripeConnection, *domain.Error) {
	items, err := s.repo.Connections(ctx, projectID)
	if err != nil {
		return nil, domain.Internal("connection_lookup_failed", "Connections could not be loaded.")
	}
	return items, nil
}

func validateCredentials(email, password string) (string, *domain.Error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	if len(normalized) < 3 || len(normalized) > 254 || !strings.Contains(normalized, "@") {
		return "", domain.Invalid("invalid_email", "A valid email is required.", "email")
	}
	if len(password) < 12 || len(password) > 256 {
		return "", domain.Invalid("weak_password", "Password must contain between 12 and 256 characters.", "password")
	}
	return normalized, nil
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=1,p=4$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}

func verifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var memory uint64
	var iterations uint64
	var threads uint64
	for _, item := range strings.Split(parts[3], ",") {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			return false
		}
		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return false
		}
		switch key {
		case "m":
			memory = parsed
		case "t":
			iterations = parsed
		case "p":
			threads = parsed
		}
	}
	if memory != 65536 || iterations != 1 || threads != 4 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) != 32 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, uint32(iterations), uint32(memory), uint8(threads), 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}

func newSessionToken() (string, [sha256.Size]byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", [sha256.Size]byte{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, sha256.Sum256([]byte(token)), nil
}

func randomUUID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

func sessionView(identity IdentityRecord, email string) SessionView {
	return SessionView{Authenticated: true, User: UserView{ID: identity.UserID, Email: email},
		Organization: OrganizationView{ID: identity.OrganizationID, Name: identity.OrganizationName, Role: identity.Role},
		Project:      ProjectView{ID: identity.ProjectID, Name: identity.ProjectName, RetentionDays: identity.RetentionDays}, ExpiresAt: identity.SessionExpiresAt}
}

func unavailable() *domain.Error {
	return &domain.Error{Type: "api_error", Code: "auth_not_configured", Message: "Authentication storage is not configured.", HTTPStatus: 503}
}
