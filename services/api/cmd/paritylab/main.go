package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/auth"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/httpapi"
	postgresadapter "github.com/ayushkumarsingh/paritylab/services/api/internal/postgres"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/secrets"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/stripeadapter"
)

func main() {
	if err := run(); err != nil {
		slog.Error("ParityLab API stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	if err := validateSandboxKey(os.Getenv("STRIPE_SECRET_KEY")); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var repository engine.Repository = engine.NewMemoryRepository()
	persistence := "memory"
	if databaseURL := os.Getenv("DATABASE_URL"); databaseURL != "" {
		opened, err := postgresadapter.Open(ctx, databaseURL, envOr("PARITYLAB_MIGRATIONS_DIR", "db/migrations"))
		if err != nil {
			return err
		}
		repository = opened
		persistence = "postgres"
	}
	service, err := engine.NewServiceWithRepository(repository)
	if err != nil {
		_ = repository.Close()
		return err
	}
	defer func() {
		if err := service.Close(); err != nil {
			slog.Warn("repository close failed", "error", err)
		}
	}()
	var secretCipher *secrets.Cipher
	if encryptionKey := os.Getenv("PARITYLAB_ENCRYPTION_KEY"); encryptionKey != "" {
		secretCipher, err = secrets.New(encryptionKey)
		if err != nil {
			return err
		}
	}
	stripeBase, err := configuredStripeAPIBase()
	if err != nil {
		return err
	}
	stripeService := engine.NewStripeService(repository, stripeadapter.New(stripeBase, nil), secretCipher)
	var authService *auth.Service
	if secretCipher != nil {
		var authRepository auth.Repository
		if postgresRepository, ok := repository.(*postgresadapter.Repository); ok {
			authRepository = postgresRepository
		} else {
			authRepository = auth.NewMemoryRepository(func(ctx context.Context, projectID string) ([]engine.StripeConnection, error) {
				tenantRepository, ok := repository.(engine.TenantRepository)
				if !ok {
					return []engine.StripeConnection{}, nil
				}
				return tenantRepository.ListStripeConnectionsForProject(ctx, projectID)
			})
		}
		authService = auth.NewService(authRepository, secretCipher)
	}

	webOrigin := envOr("WEB_ORIGIN", "http://127.0.0.1:3000")
	insecureCookies, err := configuredCookiePolicy(webOrigin)
	if err != nil {
		return err
	}
	handler := httpapi.New(service, httpapi.Config{
		WebOrigin:       webOrigin,
		WebhookSecret:   envOr("STRIPE_WEBHOOK_SECRET", "whsec_paritylab_demo"),
		EndpointToken:   envOr("STRIPE_ENDPOINT_TOKEN", "demo"),
		Stripe:          stripeService,
		Auth:            authService,
		InsecureCookies: insecureCookies,
	})
	server := &http.Server{
		Addr: envOr("API_ADDRESS", ":8080"), Handler: handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
	}
	serveError := make(chan error, 1)
	go func() {
		slog.Info("ParityLab API listening", "address", server.Addr, "mode", "sandbox", "persistence", persistence)
		serveError <- server.ListenAndServe()
	}()
	select {
	case err := <-serveError:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	}
}

func configuredCookiePolicy(webOrigin string) (bool, error) {
	if os.Getenv("PARITYLAB_INSECURE_COOKIES") != "true" {
		return false, nil
	}
	parsed, err := url.Parse(webOrigin)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() == "" {
		return false, errors.New("PARITYLAB_INSECURE_COOKIES requires an absolute loopback HTTP WEB_ORIGIN")
	}
	host := parsed.Hostname()
	if host == "localhost" {
		return true, nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return false, errors.New("PARITYLAB_INSECURE_COOKIES is allowed only for a loopback WEB_ORIGIN")
	}
	return true, nil
}

func configuredStripeAPIBase() (string, error) {
	value := strings.TrimSpace(os.Getenv("STRIPE_API_BASE"))
	if value == "" {
		return "", nil
	}
	if os.Getenv("PARITYLAB_ALLOW_STRIPE_MOCK") != "true" {
		return "", errors.New("STRIPE_API_BASE requires PARITYLAB_ALLOW_STRIPE_MOCK=true")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("STRIPE_API_BASE must be an absolute HTTP(S) URL")
	}
	return strings.TrimRight(value, "/"), nil
}

func validateSandboxKey(key string) error {
	for _, prefix := range []string{"sk_live_", "rk_live_", "pk_live_"} {
		if strings.HasPrefix(strings.TrimSpace(key), prefix) {
			return errors.New("refusing to start with a live Stripe key")
		}
	}
	return nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
