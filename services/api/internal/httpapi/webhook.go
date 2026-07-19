package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/domain"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
)

type stripeEnvelope struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Livemode bool   `json:"livemode"`
}

func (h *Handler) webhook(w http.ResponseWriter, r *http.Request) {
	if h.config.EndpointToken != "" && r.PathValue("endpoint_token") != h.config.EndpointToken {
		h.writeError(w, r, domain.NotFound("webhook_endpoint", r.PathValue("endpoint_token")))
		return
	}
	if h.config.WebhookSecret == "" {
		apiErr := &domain.Error{Type: "api_error", Code: "webhook_not_configured", Message: "Webhook verification is not configured.", HTTPStatus: http.StatusServiceUnavailable}
		h.writeError(w, r, apiErr)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err != nil {
		h.writeError(w, r, domain.Invalid("request_too_large", "The webhook payload exceeds 1 MiB.", ""))
		return
	}
	if err := verifyStripeSignature(body, r.Header.Get("Stripe-Signature"), h.config.WebhookSecret, h.config.Now(), h.config.SignatureMaxAge); err != nil {
		h.writeError(w, r, domain.Invalid("signature_verification_failed", "The webhook signature is invalid or outside the accepted time window.", "Stripe-Signature"))
		return
	}
	var event stripeEnvelope
	if err := json.Unmarshal(body, &event); err != nil || event.ID == "" || event.Type == "" {
		h.writeError(w, r, domain.Invalid("invalid_event", "The signed webhook body is not a valid Stripe event envelope.", ""))
		return
	}
	if event.Livemode {
		h.writeError(w, r, domain.Invalid("live_mode_rejected", "ParityLab v1 accepts Stripe sandbox events only.", "livemode"))
		return
	}
	duplicate, err := h.engine.RecordWebhook(event.ID, event.Type, r.PathValue("endpoint_token"), body)
	if err != nil {
		if errors.Is(err, engine.ErrWebhookConflict) {
			h.writeError(w, r, domain.Invalid("webhook_event_conflict", "This Stripe event ID was already received with different signed content.", "id"))
			return
		}
		h.writeError(w, r, domain.Internal("webhook_persistence_failed", "The verified webhook could not be durably recorded."))
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"received":  true,
		"duplicate": duplicate,
		"event":     map[string]string{"id": event.ID, "type": event.Type},
	})
}

func verifyStripeSignature(payload []byte, header, secret string, now time.Time, tolerance time.Duration) error {
	var timestamp int64
	signatures := make([]string, 0, 2)
	for _, part := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		switch key {
		case "t":
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid timestamp: %w", err)
			}
			timestamp = parsed
		case "v1":
			signatures = append(signatures, value)
		}
	}
	if timestamp == 0 || len(signatures) == 0 {
		return fmt.Errorf("required signature fields missing")
	}
	signedAt := time.Unix(timestamp, 0)
	if now.Sub(signedAt) > tolerance || signedAt.Sub(now) > tolerance {
		return fmt.Errorf("signature timestamp outside tolerance")
	}
	message := strconv.FormatInt(timestamp, 10) + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(message))
	expected := mac.Sum(nil)
	for _, candidate := range signatures {
		decoded, err := hex.DecodeString(candidate)
		if err == nil && hmac.Equal(decoded, expected) {
			return nil
		}
	}
	return fmt.Errorf("signature mismatch")
}
