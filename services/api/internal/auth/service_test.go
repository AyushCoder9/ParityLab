package auth

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/secrets"
)

func testService(t *testing.T) (*Service, *MemoryRepository) {
	t.Helper()
	cipher, err := secrets.New(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32))))
	if err != nil {
		t.Fatal(err)
	}
	repository := NewMemoryRepository(nil)
	return NewService(repository, cipher), repository
}

func TestRegisterLoginSessionLogoutLifecycle(t *testing.T) {
	service, _ := testService(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	registered, token, apiErr := service.Register(ctx, " Owner@Example.Test ", "correct-horse-battery", "QA workspace", "QA project")
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	if registered.User.Email != "owner@example.test" || registered.Project.RetentionDays != 30 || token == "" {
		t.Fatalf("unexpected registration: %#v token=%q", registered, token)
	}
	if registered.ExpiresAt != now.Add(SessionTTL) {
		t.Fatalf("expires_at=%s", registered.ExpiresAt)
	}
	loaded, apiErr := service.Session(ctx, token)
	if apiErr != nil || loaded.User.ID != registered.User.ID {
		t.Fatalf("session=%#v err=%v", loaded, apiErr)
	}

	if _, _, apiErr = service.Login(ctx, registered.User.Email, "wrong-password-value"); apiErr == nil || apiErr.Code != "invalid_credentials" {
		t.Fatalf("wrong-password error=%v", apiErr)
	}
	loggedIn, secondToken, apiErr := service.Login(ctx, registered.User.Email, "correct-horse-battery")
	if apiErr != nil || loggedIn.User.ID != registered.User.ID || secondToken == token {
		t.Fatalf("login=%#v token_reused=%v err=%v", loggedIn, secondToken == token, apiErr)
	}
	if err := service.Logout(ctx, secondToken); err != nil {
		t.Fatal(err)
	}
	if _, apiErr = service.Session(ctx, secondToken); apiErr == nil || apiErr.Code != "session_invalid" {
		t.Fatalf("revoked session error=%v", apiErr)
	}
}

func TestUnknownAndKnownAccountFailuresAreGeneric(t *testing.T) {
	service, _ := testService(t)
	ctx := context.Background()
	_, _, apiErr := service.Register(ctx, "known@example.test", "correct-horse-battery", "Workspace", "Project")
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	for _, attempt := range []struct {
		email    string
		password string
	}{
		{"missing@example.test", "correct-horse-battery"},
		{"known@example.test", "incorrect-password!"},
	} {
		_, _, got := service.Login(ctx, attempt.email, attempt.password)
		if got == nil || got.Code != "invalid_credentials" || got.Message != "The email or password is incorrect." {
			t.Fatalf("failure disclosed account state: %#v", got)
		}
	}
}

func TestDuplicateAccountAndExpiredSession(t *testing.T) {
	service, _ := testService(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 23, 11, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	_, token, apiErr := service.Register(ctx, "duplicate@example.test", "correct-horse-battery", "Workspace", "Project")
	if apiErr != nil {
		t.Fatal(apiErr)
	}
	if _, _, apiErr = service.Register(ctx, "DUPLICATE@example.test", "another-good-password", "Other", "Other"); apiErr == nil || apiErr.Code != "account_exists" {
		t.Fatalf("duplicate error=%v", apiErr)
	}
	now = now.Add(SessionTTL + time.Second)
	if _, apiErr = service.Session(ctx, token); apiErr == nil || apiErr.Code != "session_invalid" {
		t.Fatalf("expired session error=%v", apiErr)
	}
}

func TestPasswordEncodingAndDummyHashUseArgon2Parameters(t *testing.T) {
	encoded, err := hashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{"generated": encoded, "dummy": dummyPasswordHash} {
		if !strings.HasPrefix(value, "$argon2id$v=19$m=65536,t=1,p=4$") {
			t.Fatalf("%s hash parameters changed: %q", name, value)
		}
		if !verifyPassword(value, map[string]string{"generated": "correct-horse-battery", "dummy": "not-a-user-password"}[name]) {
			t.Fatalf("%s hash did not execute the verifier", name)
		}
	}
}
