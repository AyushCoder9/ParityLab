package auth

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
)

func TestInvalidEnvironmentSelectionPreservesDefault(t *testing.T) {
	repository := NewMemoryRepository(nil)
	registration := Registration{
		User:           User{ID: "user-a", EmailHash: sha256.Sum256([]byte("a"))},
		OrganizationID: "org-a", OrganizationName: "A", ProjectID: "project-a", ProjectName: "A", RetentionDays: 30,
	}
	if err := repository.Register(context.Background(), registration); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := repository.SelectEnvironment(context.Background(), "project-a", "project-b:staging", "user-a"); err != nil || ok {
		t.Fatalf("cross-tenant selection ok=%v err=%v", ok, err)
	}
	items, err := repository.Environments(context.Background(), "project-a")
	if err != nil {
		t.Fatal(err)
	}
	defaults := 0
	for _, item := range items {
		if item.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("defaults=%d after invalid selection", defaults)
	}
}

func TestTenantConnectionListerDoesNotExposeSecrets(t *testing.T) {
	engineRepository := engine.NewMemoryRepository()
	first, err := engineRepository.SaveStripeConnectionForProject(context.Background(), "project-a", engine.StripeConnection{
		ID: "11111111-1111-4111-8111-111111111111", StripeAccountID: "acct_shared",
		SecretCiphertext: []byte("secret-a"),
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := engineRepository.SaveStripeConnectionForProject(context.Background(), "project-b", engine.StripeConnection{
		ID: "22222222-2222-4222-8222-222222222222", StripeAccountID: "acct_shared",
		SecretCiphertext: []byte("secret-b"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("two projects shared one connection id")
	}
	if _, ok, _ := engineRepository.StripeConnectionForProject(context.Background(), "project-b", first.ID); ok {
		t.Fatal("project B loaded project A connection")
	}
	list, err := engineRepository.ListStripeConnectionsForProject(context.Background(), "project-a")
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%#v err=%v", list, err)
	}
	if len(list[0].SecretCiphertext) != 0 {
		t.Fatal("sanitized lister returned ciphertext")
	}
}
