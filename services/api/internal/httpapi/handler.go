package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/auth"
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
	Auth            *auth.Service
	InsecureCookies bool
	SSEPollInterval time.Duration
	SSEHeartbeat    time.Duration
	SSERetry        time.Duration
	SSEWriteTimeout time.Duration
}

type Handler struct {
	engine       *engine.Service
	config       Config
	mux          *http.ServeMux
	nextID       atomic.Uint64
	loginLimiter *loginLimiter
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
	if config.SSEPollInterval <= 0 {
		config.SSEPollInterval = 500 * time.Millisecond
	}
	if config.SSEHeartbeat <= 0 {
		config.SSEHeartbeat = 15 * time.Second
	}
	if config.SSERetry <= 0 {
		config.SSERetry = 2 * time.Second
	}
	if config.SSEWriteTimeout <= 0 {
		config.SSEWriteTimeout = 10 * time.Second
	}
	h := &Handler{engine: service, config: config, mux: http.NewServeMux(), loginLimiter: newLoginLimiter(config.Now)}
	h.routes()
	return h
}

func (h *Handler) routes() {
	h.mux.HandleFunc("GET /healthz", h.health)
	h.mux.HandleFunc("POST /v1/auth/register", h.register)
	h.mux.HandleFunc("POST /v1/auth/login", h.login)
	h.mux.HandleFunc("POST /v1/auth/logout", h.logout)
	h.mux.HandleFunc("GET /v1/session", h.session)
	h.mux.HandleFunc("GET /v1/settings/project", h.projectSettings)
	h.mux.HandleFunc("PATCH /v1/settings/project", h.updateProjectSettings)
	h.mux.HandleFunc("GET /v1/environments", h.environments)
	h.mux.HandleFunc("POST /v1/environments/{id}/select", h.selectEnvironment)
	h.mux.HandleFunc("GET /v1/findings", h.findings)
	h.mux.HandleFunc("POST /v1/findings/{id}/resolve", h.resolveFinding)
	h.mux.HandleFunc("POST /v1/findings/{id}/reopen", h.reopenFinding)
	h.mux.HandleFunc("GET /v1/notifications", h.notifications)
	h.mux.HandleFunc("POST /v1/notifications/{id}/read", h.readNotification)
	h.mux.HandleFunc("POST /v1/notifications/read-all", h.readAllNotifications)
	h.mux.HandleFunc("GET /v1/connections", h.connections)
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
	identity, ok := h.requireSession(w, r)
	if !ok && h.config.Auth != nil {
		return
	}
	var input validateStripeConnectionRequest
	body, ok := h.decodeJSONBody(w, r, &input)
	if !ok {
		return
	}
	clear(body)
	var connection engine.StripeConnection
	var apiErr *domain.Error
	if h.config.Auth != nil {
		connection, apiErr = h.config.Stripe.ValidateConnectionForProject(r.Context(), identity.Project.ID, input.SecretKey, input.SandboxName)
	} else {
		connection, apiErr = h.config.Stripe.ValidateConnection(r.Context(), input.SecretKey, input.SandboxName)
	}
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
	identity, ok := h.requireSession(w, r)
	if !ok && h.config.Auth != nil {
		return
	}
	var input createStripePaymentIntentRunRequest
	body, ok := h.decodeJSONBody(w, r, &input)
	if !ok {
		return
	}
	if input.ConnectionID == "" {
		h.writeError(w, r, domain.Invalid("parameter_missing", "The connection_id parameter is required.", "connection_id"))
		return
	}
	var run domain.Run
	var replayed bool
	var apiErr *domain.Error
	if h.config.Auth != nil {
		run, replayed, apiErr = h.config.Stripe.CreatePaymentIntentRunForProject(
			r.Context(), identity.Project.ID, input.ConnectionID, input.AmountMinor, input.Currency,
			r.Header.Get("Idempotency-Key"), body,
		)
	} else {
		run, replayed, apiErr = h.config.Stripe.CreatePaymentIntentRun(
			r.Context(), input.ConnectionID, input.AmountMinor, input.Currency,
			r.Header.Get("Idempotency-Key"), body,
		)
	}
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
	decoder := json.NewDecoder(bytes.NewReader(body))
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
	if h.config.Auth == nil || r.Header.Get("Origin") == h.config.WebOrigin {
		w.Header().Set("Access-Control-Allow-Origin", h.config.WebOrigin)
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Idempotency-Key, Last-Event-ID, Stripe-Signature")
	w.Header().Set("Access-Control-Expose-Headers", "Idempotent-Replayed, X-Request-ID")
	w.Header().Set("Vary", "Origin")
	if r.Method == http.MethodOptions {
		if h.config.Auth != nil && r.Header.Get("Origin") != h.config.WebOrigin {
			h.writeError(w, r, domain.Forbidden("cors_origin_invalid", "The request origin is not allowed."))
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if h.config.Auth != nil && r.Method != http.MethodGet && r.Method != http.MethodHead &&
		!strings.HasPrefix(r.URL.Path, "/hooks/stripe/") && r.Header.Get("Origin") != h.config.WebOrigin {
		// CSRF only matters when a cookie gives the request ambient authority
		// it wouldn't otherwise have; a request with no session cookie at all
		// carries none, so there's nothing for a forged cross-site request to
		// exploit. Blocking it anyway made anonymous/programmatic mutations
		// (the public API contract's unauthenticated run creation, for one)
		// indistinguishable from an actual CSRF attempt.
		if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
			h.writeError(w, r, domain.Forbidden("csrf_origin_invalid", "The request origin is not allowed."))
			return
		}
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

func (h *Handler) runs(w http.ResponseWriter, r *http.Request) {
	if h.config.Auth == nil {
		h.writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": h.engine.Runs(), "has_more": false})
		return
	}
	identity, authenticated, valid := h.optionalSession(w, r)
	if !valid {
		return
	}
	items := h.engine.PublicRuns()
	if authenticated {
		items = h.engine.RunsForProject(identity.Project.ID)
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": items, "has_more": false})
}

type createRunRequest struct {
	ScenarioID string       `json:"scenario_id"`
	Fault      domain.Fault `json:"fault"`
}

func (h *Handler) createRun(w http.ResponseWriter, r *http.Request) {
	var identity auth.SessionView
	var authenticated bool
	if h.config.Auth != nil {
		var valid bool
		identity, authenticated, valid = h.optionalSession(w, r)
		if !valid {
			return
		}
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBody))
	if err != nil {
		h.writeError(w, r, domain.Invalid("request_too_large", "The request body exceeds 1 MiB.", ""))
		return
	}
	var input createRunRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		h.writeError(w, r, domain.Invalid("invalid_json", "The request body must be a valid JSON object with scenario_id and optional fault.", ""))
		return
	}
	if input.ScenarioID == "" {
		h.writeError(w, r, domain.Invalid("parameter_missing", "The scenario_id parameter is required.", "scenario_id"))
		return
	}
	var run domain.Run
	var replayed bool
	var apiErr *domain.Error
	if authenticated {
		run, replayed, apiErr = h.engine.CreateRunForProject(identity.Project.ID, input.ScenarioID, input.Fault, r.Header.Get("Idempotency-Key"), body)
	} else {
		run, replayed, apiErr = h.engine.CreateRun(input.ScenarioID, input.Fault, r.Header.Get("Idempotency-Key"), body)
	}
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
	var run domain.Run
	var ok bool
	if h.config.Auth == nil {
		run, ok = h.engine.Run(id)
	} else {
		identity, authenticated, valid := h.optionalSession(w, r)
		if !valid {
			return
		}
		if authenticated {
			run, ok = h.engine.RunForProject(identity.Project.ID, id)
		} else {
			run, ok = h.engine.PublicRun(id)
		}
	}
	if !ok {
		h.writeError(w, r, domain.NotFound("run", id))
		return
	}
	h.writeJSON(w, http.StatusOK, run)
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		h.eventStream(w, r, id)
		return
	}
	var events []domain.Event
	var ok bool
	if h.config.Auth == nil {
		events, ok = h.engine.Events(id)
	} else {
		identity, authenticated, valid := h.optionalSession(w, r)
		if !valid {
			return
		}
		if authenticated {
			events, ok = h.engine.EventsForProject(identity.Project.ID, id)
		} else {
			events, ok = h.engine.PublicEvents(id)
		}
	}
	if !ok {
		h.writeError(w, r, domain.NotFound("run", id))
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": events, "has_more": false})
}

type eventBatchReader func(context.Context, int, int) (engine.EventStreamBatch, bool, error)

func (h *Handler) eventStream(w http.ResponseWriter, r *http.Request, id string) {
	reader, valid := h.eventReader(w, r, id)
	if !valid {
		return
	}
	lastSequence, apiErr := parseLastEventID(r.Header.Values("Last-Event-ID"))
	if apiErr != nil {
		h.writeError(w, r, apiErr)
		return
	}
	const batchSize = 100
	batch, found, err := reader(r.Context(), lastSequence, batchSize)
	if err != nil {
		h.writeError(w, r, domain.Internal("event_stream_unavailable", "The event stream could not be opened."))
		return
	}
	if !found {
		h.writeError(w, r, domain.NotFound("run", id))
		return
	}
	if lastSequence > batch.HighWater {
		h.writeError(w, r, domain.Invalid(
			"last_event_id_ahead",
			"Last-Event-ID is ahead of the latest event for this run.",
			"Last-Event-ID",
		))
		return
	}
	_, canFlush := w.(http.Flusher)
	if !canFlush {
		h.writeError(w, r, domain.Internal("streaming_unsupported", "Streaming is unavailable for this response."))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	if err := refreshSSEWriteDeadline(w, h.config.SSEWriteTimeout); err != nil {
		h.writeError(w, r, domain.Internal("streaming_unavailable", "Streaming is unavailable for this response."))
		return
	}
	w.WriteHeader(http.StatusOK)
	retryMilliseconds := h.config.SSERetry.Milliseconds()
	if retryMilliseconds < 1 {
		retryMilliseconds = 1
	}
	if err := refreshSSEWriteDeadline(w, h.config.SSEWriteTimeout); err != nil {
		return
	}
	if _, err := fmt.Fprintf(w, "retry: %d\n\n", retryMilliseconds); err != nil {
		return
	}
	if err := flushSSE(w, h.config.SSEWriteTimeout); err != nil {
		return
	}

	completeSent := false
	for {
		next, writeErr := writeEventBatch(w, batch, lastSequence, h.config.SSEWriteTimeout)
		if writeErr != nil {
			return
		}
		lastSequence = next
		if err := flushSSE(w, h.config.SSEWriteTimeout); err != nil {
			return
		}
		if len(batch.Events) < batchSize {
			break
		}
		batch, found, err = reader(r.Context(), lastSequence, batchSize)
		if err != nil || !found {
			writeStreamError(w, h.config.SSEWriteTimeout)
			return
		}
	}
	if isTerminalRun(batch.Run) {
		if err := refreshSSEWriteDeadline(w, h.config.SSEWriteTimeout); err != nil {
			return
		}
		if _, err := fmt.Fprintf(w, "event: run.complete\ndata: {\"run_id\":%q}\n\n", id); err != nil {
			return
		}
		completeSent = true
		if err := flushSSE(w, h.config.SSEWriteTimeout); err != nil {
			return
		}
	}

	poll := time.NewTicker(h.config.SSEPollInterval)
	defer poll.Stop()
	heartbeat := time.NewTicker(h.config.SSEHeartbeat)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-poll.C:
			batch, found, err = reader(r.Context(), lastSequence, batchSize)
			if err != nil || !found {
				writeStreamError(w, h.config.SSEWriteTimeout)
				return
			}
			for {
				next, writeErr := writeEventBatch(w, batch, lastSequence, h.config.SSEWriteTimeout)
				if writeErr != nil {
					return
				}
				lastSequence = next
				if !completeSent && isTerminalRun(batch.Run) {
					if err := refreshSSEWriteDeadline(w, h.config.SSEWriteTimeout); err != nil {
						return
					}
					if _, err := fmt.Fprintf(w, "event: run.complete\ndata: {\"run_id\":%q}\n\n", id); err != nil {
						return
					}
					completeSent = true
				}
				if err := flushSSE(w, h.config.SSEWriteTimeout); err != nil {
					return
				}
				if len(batch.Events) < batchSize {
					break
				}
				batch, found, err = reader(r.Context(), lastSequence, batchSize)
				if err != nil || !found {
					writeStreamError(w, h.config.SSEWriteTimeout)
					return
				}
			}
		case <-heartbeat.C:
			if err := refreshSSEWriteDeadline(w, h.config.SSEWriteTimeout); err != nil {
				return
			}
			if _, err := io.WriteString(w, ": heartbeat\n\n"); err != nil {
				return
			}
			if err := flushSSE(w, h.config.SSEWriteTimeout); err != nil {
				return
			}
		}
	}
}

func (h *Handler) eventReader(w http.ResponseWriter, r *http.Request, id string) (eventBatchReader, bool) {
	if h.config.Auth == nil {
		return func(ctx context.Context, after, limit int) (engine.EventStreamBatch, bool, error) {
			return h.engine.EventsAfter(ctx, id, after, limit)
		}, true
	}
	identity, authenticated, valid := h.optionalSession(w, r)
	if !valid {
		return nil, false
	}
	if authenticated {
		return func(ctx context.Context, after, limit int) (engine.EventStreamBatch, bool, error) {
			return h.engine.EventsAfterForProject(ctx, identity.Project.ID, id, after, limit)
		}, true
	}
	return func(ctx context.Context, after, limit int) (engine.EventStreamBatch, bool, error) {
		return h.engine.PublicEventsAfter(ctx, id, after, limit)
	}, true
}

func parseLastEventID(values []string) (int, *domain.Error) {
	if len(values) == 0 {
		return 0, nil
	}
	if len(values) != 1 {
		return 0, domain.Invalid("invalid_last_event_id", "Last-Event-ID must contain one decimal event sequence.", "Last-Event-ID")
	}
	value := values[0]
	if value == "" || (len(value) > 1 && value[0] == '0') {
		return 0, domain.Invalid("invalid_last_event_id", "Last-Event-ID must contain one decimal event sequence.", "Last-Event-ID")
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, domain.Invalid("invalid_last_event_id", "Last-Event-ID must contain one decimal event sequence.", "Last-Event-ID")
		}
	}
	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, domain.Invalid("invalid_last_event_id", "Last-Event-ID must contain one decimal event sequence.", "Last-Event-ID")
	}
	return int(parsed), nil
}

func writeEventBatch(w http.ResponseWriter, batch engine.EventStreamBatch, after int, writeTimeout time.Duration) (int, error) {
	last := after
	for _, event := range batch.Events {
		if event.Sequence <= last {
			continue
		}
		payload, err := json.Marshal(event)
		if err != nil {
			return last, err
		}
		if err := refreshSSEWriteDeadline(w, writeTimeout); err != nil {
			return last, err
		}
		if _, err := fmt.Fprintf(w, "id: %d\nevent: run.event\ndata: %s\n\n", event.Sequence, payload); err != nil {
			return last, err
		}
		last = event.Sequence
	}
	return last, nil
}

func isTerminalRun(run domain.Run) bool {
	return run.Status == domain.RunPassed || run.Status == domain.RunFailed
}

func writeStreamError(w http.ResponseWriter, writeTimeout time.Duration) {
	if err := refreshSSEWriteDeadline(w, writeTimeout); err != nil {
		return
	}
	_, _ = io.WriteString(w, "event: stream.error\ndata: {\"code\":\"stream_unavailable\"}\n\n")
	_ = http.NewResponseController(w).Flush()
}

func refreshSSEWriteDeadline(w http.ResponseWriter, timeout time.Duration) error {
	err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(timeout))
	if errors.Is(err, http.ErrNotSupported) {
		return nil
	}
	return err
}

func flushSSE(w http.ResponseWriter, writeTimeout time.Duration) error {
	if err := refreshSSEWriteDeadline(w, writeTimeout); err != nil {
		return err
	}
	return http.NewResponseController(w).Flush()
}

func (h *Handler) report(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var report domain.Report
	var ok bool
	if h.config.Auth == nil {
		report, ok = h.engine.Report(id)
	} else {
		identity, authenticated, valid := h.optionalSession(w, r)
		if !valid {
			return
		}
		if authenticated {
			report, ok = h.engine.ReportForProject(identity.Project.ID, id)
		} else {
			report, ok = h.engine.PublicReport(id)
		}
	}
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
