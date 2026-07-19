package secrets

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestCipherRoundTripAndAssociatedData(t *testing.T) {
	t.Parallel()
	key := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))
	cipher, err := New(key)
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := cipher.Encrypt([]byte("sk_test_never_plaintext"), "conn-a")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encrypted), "sk_test") {
		t.Fatal("ciphertext contains plaintext")
	}
	decrypted, err := cipher.Decrypt(encrypted, "conn-a")
	if err != nil || string(decrypted) != "sk_test_never_plaintext" {
		t.Fatalf("decrypted=%q err=%v", decrypted, err)
	}
	if _, err := cipher.Decrypt(encrypted, "conn-b"); err == nil {
		t.Fatal("ciphertext decrypted with wrong associated data")
	}
}
