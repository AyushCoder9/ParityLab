package contracts_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

type webhookResponse struct {
	Received  bool `json:"received"`
	Duplicate bool `json:"duplicate"`
}

func TestSignedWebhookDeduplicates(t *testing.T) {
	baseURL := os.Getenv("PARITYLAB_CONTRACT_API_URL")
	if baseURL == "" {
		t.Skip("set PARITYLAB_CONTRACT_API_URL to run integration contracts")
	}

	body := []byte(`{"id":"evt_contract_duplicate","object":"event","type":"payment_intent.succeeded","livemode":false,"data":{"object":{"id":"pi_contract"}}}`)
	first := deliverWebhook(t, baseURL, body, "whsec_paritylab_demo")
	second := deliverWebhook(t, baseURL, body, "whsec_paritylab_demo")
	if !first.Received || first.Duplicate {
		t.Fatalf("first delivery = %+v, want received and not duplicate", first)
	}
	if !second.Received || !second.Duplicate {
		t.Fatalf("second delivery = %+v, want received and duplicate", second)
	}
}

func TestWebhookRejectsInvalidSignature(t *testing.T) {
	baseURL := os.Getenv("PARITYLAB_CONTRACT_API_URL")
	if baseURL == "" {
		t.Skip("set PARITYLAB_CONTRACT_API_URL to run integration contracts")
	}

	body := []byte(`{"id":"evt_contract_invalid","object":"event","type":"payment_intent.succeeded","livemode":false}`)
	req, err := http.NewRequest(http.MethodPost, baseURL+"/hooks/stripe/demo", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Signature", fmt.Sprintf("t=%d,v1=invalid", time.Now().Unix()))
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, want 400; body=%s", response.StatusCode, payload)
	}
}

func deliverWebhook(t *testing.T, baseURL string, body []byte, secret string) webhookResponse {
	t.Helper()
	timestamp := time.Now().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "%d.", timestamp)
	_, _ = mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequest(http.MethodPost, baseURL+"/hooks/stripe/demo", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Stripe-Signature", fmt.Sprintf("t=%d,v1=%s", timestamp, signature))
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, want 200; body=%s", response.StatusCode, payload)
	}
	var result webhookResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}
