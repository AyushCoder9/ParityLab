package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/domain"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
)

const maxRequestBody = 1 << 20

type Config struct {
	WebOrigin       string
	WebhookSecret   string
	EndpointToken   string
	SignatureMaxAge time.Duration
	Now             func() time.Time
	Stripe          *engine.StripeService
}

type Handler struct {
	engine *engine.Service
	config Config
	mux    *http.ServeMux
	nextID atomic.Uint64
}

func New(service *engine.Service, config Config) http.Handler {
	if config.WebOrigin == "" {
		config.WebOrigin = "http://localhost:3000"
	}
	if config.SignatureMaxAge == 0 {
		config.SignatureMaxAge = 5 * time.Minute
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	h := &Handler{engine: service, config: config, mux: http.NewServeMux()}
	h.routes()
	return h
}

func (h *Handler) routes() {
	h.mux.HandleFunc("GET /healthz", h.health)
	h.mux.HandleFunc("GET /v1/overview", h.overview)
	h.mux.HandleFunc("GET /v1/scenarios", h.scenarios)
	h.mux.HandleFunc("GET /v1/runs", h.runs)
	h.mux.HandleFunc("POST /v1/runs", h.createRun)
	h.mux.HandleFunc("POST /v1/stripe/payment-intents", h.createStripePaymentIntentRun)
	h.mux.HandleFunc("POST /v1/connections/stripe/validate", h.validateStripeConnection)
	h.mux.HandleFunc("GET /v1/runs/{id}", h.run)
	h.mux.HandleFunc("GET /v1/runs/{id}/events", h.events)
	h.mux.HandleFunc("GET /v1/runs/{id}/report", h.report)
	h.mux.HandleFunc("POST /hooks/stripe/{endpoint_token}", h.webhook)
}

type validateStripeConnectionRequest struct {
	SecretKey   string `json:"secret_key"`
	SandboxName string `json:"sandbox_name"`
}

func (h *Handler) validateStripeConnection(w http.ResponseWriter, r *http.Request) {
	var input validateStripeConnectionRequest
	body, ok := h.decodeJSONBody(w, r, &input)
	if !ok {
		return
	}
	clear(body)
	connection, apiErr := h.config.Stripe.ValidateConnection(r.Context(), input.SecretKey, input.SandboxName)
	input.SecretKey = ""
	if apiErr != nil {
		h.writeError(w, r, apiErr)
		return
	}
	h.writeJSON(w, http.StatusCreated, connection)
}

type createStripePaymentIntentRunRequest struct {
	ConnectionID string `json:"connection_id"`
	AmountMinor  int64  `json:"amount_minor"`
	Currency     string `json:"currency"`
}

func (h *Handler) createStripePaymentIntentRun(w http.ResponseWriter, r *http.Request) {
	var input createStripePaymentIntentRunRequest
	body, ok := h.decodeJSONBody(w, r, &input)
	if !ok {
		return
	}
	if input.ConnectionID == "" {
		h.writeError(w, r, domain.Invalid("parameter_missing", "The connection_id parameter is required.", "connection_id"))
		return
	}
	run, replayed, apiErr := h.config.Stripe.CreatePaymentIntentRun(
		r.Context(), input.ConnectionID, input.AmountMinor, input.Currency,
		r.Header.Get("Idempotency-Key"), body,
	)
	if apiErr != nil {
		h.writeError(w, r, apiErr)
		return
	}
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	h.writeJSON(w, http.StatusCreated, run)
}

func (h *Handler) decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) ([]byte, bool) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err != nil {
		h.writeError(w, r, domain.Invalid("request_too_large", "The request body exceeds 1 MiB.", ""))
		return nil, false
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		h.writeError(w, r, domain.Invalid("invalid_json", "The request body must be a valid JSON object with only supported fields.", ""))
		return nil, false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		h.writeError(w, r, domain.Invalid("invalid_json", "The request body must contain exactly one JSON object.", ""))
		return nil, false
	}
	return body, true
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := fmt.Sprintf("req_%016x", h.nextID.Add(1))
	r.Header.Set("X-ParityLab-Request-ID", requestID)
	w.Header().Set("X-Request-ID", requestID)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Access-Control-Allow-Origin", h.config.WebOrigin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Idempotency-Key, Stripe-Signature")
	w.Header().Set("Access-Control-Expose-Headers", "Idempotent-Replayed, X-Request-ID")
	w.Header().Set("Vary", "Origin")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "paritylab-api", "mode": "sandbox"})
}

func (h *Handler) overview(w http.ResponseWriter, _ *http.Request) {
	h.writeJSON(w, http.StatusOK, h.engine.Overview())
}

func (h *Handler) scenarios(w http.ResponseWriter, _ *http.Request) {
	items := h.engine.Scenarios()
	h.writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": items, "has_more": false})
}

func (h *Handler) runs(w http.ResponseWriter, _ *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": h.engine.Runs(), "has_more": false})
}

type createRunRequest struct {
	ScenarioID string       `json:"scenario_id"`
	Fault      domain.Fault `json:"fault"`
}

func (h *Handler) createRun(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err != nil {
		h.writeError(w, r, domain.Invalid("request_too_large", "The request body exceeds 1 MiB.", ""))
		return
	}
	var input createRunRequest
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		h.writeError(w, r, domain.Invalid("invalid_json", "The request body must be a valid JSON object with scenario_id and optional fault.", ""))
		return
	}
	if input.ScenarioID == "" {
		h.writeError(w, r, domain.Invalid("parameter_missing", "The scenario_id parameter is required.", "scenario_id"))
		return
	}
	run, replayed, apiErr := h.engine.CreateRun(input.ScenarioID, input.Fault, r.Header.Get("Idempotency-Key"), body)
	if apiErr != nil {
		h.writeError(w, r, apiErr)
		return
	}
	if replayed {
		w.Header().Set("Idempotent-Replayed", "true")
	}
	h.writeJSON(w, http.StatusCreated, run)
}

func (h *Handler) run(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	run, ok := h.engine.Run(id)
	if !ok {
		h.writeError(w, r, domain.NotFound("run", id))
		return
	}
	h.writeJSON(w, http.StatusOK, run)
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	events, ok := h.engine.Events(id)
	if !ok {
		h.writeError(w, r, domain.NotFound("run", id))
		return
	}
	if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		h.writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": events, "has_more": false})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, canFlush := w.(http.Flusher)
	for _, event := range events {
		payload, err := json.Marshal(event)
		if err != nil {
			continue
		}
		_, _ = fmt.Fprintf(w, "id: %d\nevent: run.event\ndata: %s\n\n", event.Sequence, payload)
		if canFlush {
			flusher.Flush()
		}
	}
	_, _ = fmt.Fprintf(w, "event: run.complete\ndata: {\"run_id\":%q}\n\n", id)
	if canFlush {
		flusher.Flush()
	}
}

func (h *Handler) report(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	report, ok := h.engine.Report(id)
	if !ok {
		h.writeError(w, r, domain.NotFound("run", id))
		return
	}
	h.writeJSON(w, http.StatusOK, report)
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, apiErr *domain.Error) {
	apiErr.RequestID = r.Header.Get("X-ParityLab-Request-ID")
	status := apiErr.HTTPStatus
	if status == 0 {
		status = http.StatusInternalServerError
	}
	h.writeJSON(w, status, map[string]any{"error": apiErr})
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil && !errors.Is(err, http.ErrHandlerTimeout) {
		return
	}
}
