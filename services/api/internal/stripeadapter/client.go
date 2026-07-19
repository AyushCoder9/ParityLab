package stripeadapter

import (
	"context"
	"net/http"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
	stripe "github.com/stripe/stripe-go/v86"
)

const productionAPIBase = "https://api.stripe.com"

// Client is a narrow adapter around Stripe's current official Go SDK.
type Client struct {
	apiBase    string
	httpClient *http.Client
}

func New(apiBase string, httpClient *http.Client) *Client {
	if apiBase == "" {
		apiBase = productionAPIBase
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{apiBase: apiBase, httpClient: httpClient}
}

func (c *Client) sdk(secret string) *stripe.Client {
	config := &stripe.BackendConfig{
		URL: stripe.String(c.apiBase), HTTPClient: c.httpClient,
		EnableTelemetry: stripe.Bool(false), MaxNetworkRetries: stripe.Int64(2),
		LeveledLogger: &stripe.LeveledLogger{Level: stripe.LevelNull},
	}
	return stripe.NewClient(secret, stripe.WithBackends(stripe.NewBackendsWithConfig(config)))
}

func (c *Client) ValidateAccount(ctx context.Context, secret string) (engine.StripeAccount, error) {
	if err := engine.ValidateSandboxSecret(secret); err != nil {
		return engine.StripeAccount{}, err
	}
	account, err := c.sdk(secret).V1Accounts.Retrieve(ctx, nil)
	if err != nil {
		return engine.StripeAccount{}, err
	}
	return engine.StripeAccount{ID: account.ID}, nil
}

func (c *Client) CreatePaymentIntent(ctx context.Context, secret string, input engine.StripePaymentIntentParams) (engine.StripePaymentIntent, error) {
	if err := engine.ValidateSandboxSecret(secret); err != nil {
		return engine.StripePaymentIntent{}, err
	}
	params := &stripe.PaymentIntentCreateParams{
		Amount: stripe.Int64(input.AmountMinor), Currency: stripe.String(input.Currency),
		Metadata: input.Metadata,
	}
	params.SetIdempotencyKey(input.IdempotencyKey)
	intent, err := c.sdk(secret).V1PaymentIntents.Create(ctx, params)
	if err != nil {
		return engine.StripePaymentIntent{}, err
	}
	return engine.StripePaymentIntent{
		ID: intent.ID, Status: string(intent.Status), Amount: intent.Amount, Currency: string(intent.Currency),
	}, nil
}
