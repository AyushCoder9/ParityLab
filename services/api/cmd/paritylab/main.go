package main

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/httpapi"
)

func main() {
	address := envOr("API_ADDRESS", ":8080")
	secret := envOr("STRIPE_WEBHOOK_SECRET", "whsec_paritylab_demo")
	if strings.HasPrefix(os.Getenv("STRIPE_SECRET_KEY"), "sk_live_") {
		slog.Error("refusing to start with a live Stripe key")
		os.Exit(1)
	}

	handler := httpapi.New(engine.NewService(), httpapi.Config{
		WebOrigin:     envOr("WEB_ORIGIN", "http://127.0.0.1:3000"),
		WebhookSecret: secret,
		EndpointToken: envOr("STRIPE_ENDPOINT_TOKEN", "demo"),
	})
	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	slog.Info("ParityLab API listening", "address", address, "mode", "sandbox")
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("API server stopped", "error", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
