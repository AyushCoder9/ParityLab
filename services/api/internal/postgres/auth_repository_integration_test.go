package postgres

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/auth"
)

func TestAuthRepositorySwitchesDefaultEnvironmentWithoutUniqueViolation(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	repository, err := Open(ctx, databaseURL, "../../../../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	defer repository.Close()

	suffix := fmt.Sprintf("%012x", time.Now().UnixNano()&0xffffffffffff)
	userID := "10000000-0000-4000-8000-" + suffix
	organizationID := "20000000-0000-4000-8000-" + suffix
	projectID := "30000000-0000-4000-8000-" + suffix
	registration := auth.Registration{
		User: auth.User{
			ID: userID, EmailHash: sha256.Sum256([]byte(suffix)),
			EmailCiphertext: []byte("ciphertext"), PasswordHash: "password-hash",
		},
		OrganizationID: organizationID, OrganizationName: "Integration",
		ProjectID: projectID, ProjectName: "Integration", RetentionDays: 30,
		Session: auth.SessionRecord{
			TokenHash: sha256.Sum256([]byte("session-" + suffix)), UserID: userID,
			ProjectID: projectID, ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	if err := repository.Register(ctx, registration); err != nil {
		t.Fatal(err)
	}
	environments, err := repository.Environments(ctx, projectID)
	if err != nil {
		t.Fatal(err)
	}
	for _, environment := range environments {
		if environment.Kind != "staging" {
			continue
		}
		selected, ok, err := repository.SelectEnvironment(ctx, projectID, environment.ID, userID)
		if err != nil || !ok || !selected.IsDefault {
			t.Fatalf("selected=%#v ok=%v err=%v", selected, ok, err)
		}
		again, ok, err := repository.SelectEnvironment(ctx, projectID, environment.ID, userID)
		if err != nil || !ok || !again.IsDefault {
			t.Fatalf("repeat selected=%#v ok=%v err=%v", again, ok, err)
		}
		return
	}
	t.Fatal("staging environment not seeded")
}
