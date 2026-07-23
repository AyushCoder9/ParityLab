package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	postgresadapter "github.com/ayushkumarsingh/paritylab/services/api/internal/postgres"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/verification"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/worker"
)

func main() {
	if err := run(); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("ParityLab worker stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return errors.New("DATABASE_URL is required for the durable worker")
	}
	signer, err := verification.NewSigner(os.Getenv("PARITYLAB_SIGNING_SECRET"))
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if port := os.Getenv("PORT"); port != "" {
		// Render's free tier only offers "Web Service" instances; a background
		// worker has no HTTP-serving requirement of its own but binds $PORT so
		// the platform treats this process as a Web Service instead of the
		// paid-only Background Worker type.
		go func() {
			mux := http.NewServeMux()
			mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			})
			if err := http.ListenAndServe(":"+port, mux); err != nil {
				slog.Warn("worker health server stopped", "error", err)
			}
		}()
	}
	repository, err := postgresadapter.Open(ctx, databaseURL, envOr("PARITYLAB_MIGRATIONS_DIR", "db/migrations"))
	if err != nil {
		return err
	}
	defer repository.Close()
	hostname, _ := os.Hostname()
	workerID := envOr("PARITYLAB_WORKER_ID", fmt.Sprintf("%s-%d", hostname, os.Getpid()))
	merchant := worker.NewRepositoryMerchant(repository, signer)
	service := worker.New(repository, verification.NewRelay(signer, merchant), worker.Config{ID: workerID})
	slog.Info("ParityLab worker started", "worker_id", workerID, "contract", verification.ContractVersion)
	return service.Run(ctx)
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
