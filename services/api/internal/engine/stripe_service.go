package engine

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/domain"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/secrets"
)

type StripeAccount struct{ ID string }

type StripePaymentIntent struct {
	ID       string
	Status   string
	Amount   int64
	Currency string
}

type StripePaymentIntentParams struct {
	AmountMinor    int64
	Currency       string
	IdempotencyKey string
	Metadata       map[string]string
}

type StripeGateway interface {
	ValidateAccount(context.Context, string) (StripeAccount, error)
	CreatePaymentIntent(context.Context, string, StripePaymentIntentParams) (StripePaymentIntent, error)
}

type StripeService struct {
	repo    Repository
	gateway StripeGateway
	cipher  *secrets.Cipher
	now     func() time.Time
}

func NewStripeService(repo Repository, gateway StripeGateway, cipher *secrets.Cipher) *StripeService {
	return &StripeService{repo: repo, gateway: gateway, cipher: cipher, now: time.Now}
}

func ValidateSandboxSecret(secret string) error {
	secret = strings.TrimSpace(secret)
	if strings.HasPrefix(secret, "sk_live_") || strings.HasPrefix(secret, "rk_live_") || strings.HasPrefix(secret, "pk_live_") {
		return errors.New("live Stripe keys are not supported")
	}
	if !strings.HasPrefix(secret, "sk_test_") && !strings.HasPrefix(secret, "rk_test_") {
		return errors.New("a Stripe sandbox secret or restricted key is required")
	}
	return nil
}

func (s *StripeService) ValidateConnection(ctx context.Context, secret, sandboxName string) (StripeConnection, *domain.Error) {
	return s.validateConnection(ctx, "", secret, sandboxName)
}

func (s *StripeService) ValidateConnectionForProject(ctx context.Context, projectID, secret, sandboxName string) (StripeConnection, *domain.Error) {
	if projectID == "" {
		return StripeConnection{}, domain.Forbidden("project_required", "A project session is required.")
	}
	return s.validateConnection(ctx, projectID, secret, sandboxName)
}

func (s *StripeService) validateConnection(ctx context.Context, projectID, secret, sandboxName string) (StripeConnection, *domain.Error) {
	if s == nil || s.gateway == nil || s.cipher == nil {
		return StripeConnection{}, &domain.Error{Type: "api_error", Code: "connection_storage_not_configured", Message: "Encrypted Stripe connection storage is not configured.", HTTPStatus: 503}
	}
	if err := ValidateSandboxSecret(secret); err != nil {
		return StripeConnection{}, domain.Invalid("sandbox_key_required", err.Error(), "secret_key")
	}
	account, err := s.gateway.ValidateAccount(ctx, strings.TrimSpace(secret))
	if err != nil || !strings.HasPrefix(account.ID, "acct_") {
		return StripeConnection{}, domain.Invalid("stripe_connection_failed", "Stripe could not validate this sandbox key.", "secret_key")
	}
	if strings.TrimSpace(sandboxName) == "" {
		sandboxName = "Stripe Sandbox"
	}
	id, err := newUUID()
	if err != nil {
		return StripeConnection{}, domain.Internal("connection_id_failed", "The connection could not be created.")
	}
	ciphertext, err := s.cipher.Encrypt([]byte(strings.TrimSpace(secret)), account.ID)
	if err != nil {
		return StripeConnection{}, domain.Internal("connection_encryption_failed", "The connection secret could not be encrypted.")
	}
	connection := StripeConnection{
		ID: id, StripeAccountID: account.ID, SandboxName: strings.TrimSpace(sandboxName),
		Status: "connected", CreatedAt: s.now().UTC(), SecretCiphertext: ciphertext,
		SecretEncryptionKeyID: 1,
	}
	save := s.repo.SaveStripeConnection
	if projectID != "" {
		tenantRepo, ok := s.repo.(TenantRepository)
		if !ok {
			return StripeConnection{}, domain.Internal("tenant_storage_unavailable", "Project-scoped connection storage is unavailable.")
		}
		save = func(ctx context.Context, connection StripeConnection) (StripeConnection, error) {
			return tenantRepo.SaveStripeConnectionForProject(ctx, projectID, connection)
		}
	}
	stored, err := save(ctx, connection)
	if err != nil {
		return StripeConnection{}, domain.Internal("connection_persistence_failed", "The validated connection could not be stored.")
	}
	return stored, nil
}

var currencyPattern = regexp.MustCompile(`^[a-z]{3}$`)

func (s *StripeService) CreatePaymentIntentRun(ctx context.Context, connectionID string, amountMinor int64, currency, idempotencyKey string, requestBody []byte) (domain.Run, bool, *domain.Error) {
	return s.createPaymentIntentRun(ctx, "", connectionID, amountMinor, currency, idempotencyKey, requestBody)
}

func (s *StripeService) CreatePaymentIntentRunForProject(ctx context.Context, projectID, connectionID string, amountMinor int64, currency, idempotencyKey string, requestBody []byte) (domain.Run, bool, *domain.Error) {
	if projectID == "" {
		return domain.Run{}, false, domain.Forbidden("project_required", "A project session is required.")
	}
	return s.createPaymentIntentRun(ctx, projectID, connectionID, amountMinor, currency, idempotencyKey, requestBody)
}

func (s *StripeService) createPaymentIntentRun(ctx context.Context, projectID, connectionID string, amountMinor int64, currency, idempotencyKey string, requestBody []byte) (domain.Run, bool, *domain.Error) {
	if s == nil || s.gateway == nil || s.cipher == nil {
		return domain.Run{}, false, &domain.Error{Type: "api_error", Code: "connection_storage_not_configured", Message: "Encrypted Stripe connection storage is not configured.", HTTPStatus: 503}
	}
	if idempotencyKey == "" {
		return domain.Run{}, false, domain.Invalid("idempotency_key_missing", "An Idempotency-Key header is required for this request.", "Idempotency-Key")
	}
	if amountMinor <= 0 || amountMinor > 99_999_999 {
		return domain.Run{}, false, domain.Invalid("invalid_amount_minor", "amount_minor must be an integer between 1 and 99999999.", "amount_minor")
	}
	if !currencyPattern.MatchString(currency) {
		return domain.Run{}, false, domain.Invalid("invalid_currency", "currency must be a lowercase three-letter ISO code.", "currency")
	}
	keyHash := sha256.Sum256([]byte(idempotencyKey))
	requestHash := sha256.Sum256(requestBody)
	replayRun := s.repo.ReplayRun
	createRun := s.repo.CreateRun
	loadConnection := s.repo.StripeConnection
	if projectID != "" {
		tenantRepo, ok := s.repo.(TenantRepository)
		if !ok {
			return domain.Run{}, false, domain.Internal("tenant_storage_unavailable", "Project-scoped Stripe storage is unavailable.")
		}
		replayRun = func(ctx context.Context, keyHash, requestHash [sha256.Size]byte) (domain.Run, bool, error) {
			return tenantRepo.ReplayRunForProject(ctx, projectID, keyHash, requestHash)
		}
		createRun = func(ctx context.Context, keyHash, requestHash [sha256.Size]byte, bundle RunBundle) (domain.Run, bool, error) {
			return tenantRepo.CreateRunForProject(ctx, projectID, keyHash, requestHash, bundle)
		}
		loadConnection = func(ctx context.Context, id string) (StripeConnection, bool, error) {
			return tenantRepo.StripeConnectionForProject(ctx, projectID, id)
		}
	}
	if replay, found, err := replayRun(ctx, keyHash, requestHash); err != nil {
		if errors.Is(err, ErrIdempotencyConflict) {
			return domain.Run{}, false, domain.Conflict("idempotency_key_in_use", "This idempotency key was already used with different request parameters.", "Idempotency-Key")
		}
		return domain.Run{}, false, domain.Internal("persistence_failed", "The idempotency record could not be read.")
	} else if found {
		return replay, true, nil
	}
	if !uuidPattern.MatchString(connectionID) {
		return domain.Run{}, false, domain.NotFound("stripe_connection", connectionID)
	}
	connection, ok, err := loadConnection(ctx, connectionID)
	if err != nil {
		return domain.Run{}, false, domain.Internal("connection_lookup_failed", "The Stripe connection could not be loaded.")
	}
	if !ok || connection.Status != "connected" {
		return domain.Run{}, false, domain.NotFound("stripe_connection", connectionID)
	}
	secret, err := s.cipher.Decrypt(connection.SecretCiphertext, connection.StripeAccountID)
	if err != nil {
		return domain.Run{}, false, domain.Internal("connection_decryption_failed", "The Stripe connection could not be decrypted.")
	}
	defer clear(secret)
	if err := ValidateSandboxSecret(string(secret)); err != nil {
		return domain.Run{}, false, domain.Invalid("sandbox_key_required", "The stored Stripe connection is not a sandbox key.", "connection_id")
	}
	id, err := s.repo.NextRunID(ctx)
	if err != nil {
		return domain.Run{}, false, domain.Internal("persistence_unavailable", "The run could not be durably allocated.")
	}
	stripeIdempotency := sha256.Sum256([]byte("paritylab:stripe:" + idempotencyKey))
	correlationID := "plcorr_" + hex.EncodeToString(stripeIdempotency[:12])
	intent, err := s.gateway.CreatePaymentIntent(ctx, string(secret), StripePaymentIntentParams{
		AmountMinor: amountMinor, Currency: currency,
		IdempotencyKey: "pl_" + hex.EncodeToString(stripeIdempotency[:]),
		Metadata: map[string]string{
			"paritylab_correlation_id": correlationID, "paritylab_scenario_id": "checkout-duplicate",
		},
	})
	if err != nil || !strings.HasPrefix(intent.ID, "pi_") {
		return domain.Run{}, false, &domain.Error{Type: "api_error", Code: "stripe_payment_intent_failed", Message: "Stripe could not create the sandbox PaymentIntent.", HTTPStatus: 502}
	}
	scenario, _ := scenarioByID(seededScenarios(), "checkout-duplicate")
	bundle := buildRunBundle(id, numberFromRunID(id), scenario, domain.FaultDuplicate, s.now().UTC())
	bundle.OutboxTopic = "verification.run.queued"
	bundle.Run.StripeObjectID = intent.ID
	for index := range bundle.Events {
		bundle.Events[index].Evidence["amount_minor"] = amountMinor
		bundle.Events[index].Evidence["currency"] = currency
		bundle.Events[index].Evidence["stripe_payment_intent_id"] = intent.ID
		bundle.Events[index].Evidence["paritylab_correlation_id"] = correlationID
	}
	bundle.Report = buildReport(bundle.Run, domain.FaultDuplicate)
	bundle.Report.Assertions = append(bundle.Report.Assertions, domain.Assertion{
		ID: "assert_minor_units", Name: "Amount persisted in integer minor units", Passed: intent.Amount == amountMinor && intent.Currency == currency,
		Expected: fmt.Sprintf("%d %s", amountMinor, currency), Observed: fmt.Sprintf("%d %s", intent.Amount, intent.Currency), Evidence: intent.ID,
	})
	bundle.Report.Summary = fmt.Sprintf("%d of %d deterministic assertions passed.", passedCount(bundle.Report.Assertions), len(bundle.Report.Assertions))
	run, replayed, err := createRun(ctx, keyHash, requestHash, bundle)
	if errors.Is(err, ErrIdempotencyConflict) {
		return domain.Run{}, false, domain.Conflict("idempotency_key_in_use", "This idempotency key was already used with different request parameters.", "Idempotency-Key")
	}
	if err != nil {
		return domain.Run{}, false, domain.Internal("persistence_failed", "The Stripe run could not be durably persisted.")
	}
	return run, replayed, nil
}

func scenarioByID(scenarios []domain.Scenario, id string) (domain.Scenario, bool) {
	for _, scenario := range scenarios {
		if scenario.ID == id {
			return scenario, true
		}
	}
	return domain.Scenario{}, false
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func newUUID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
