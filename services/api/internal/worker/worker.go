package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/domain"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/engine"
	"github.com/ayushkumarsingh/paritylab/services/api/internal/verification"
)

type Config struct {
	ID          string
	Lease       time.Duration
	Poll        time.Duration
	MaxAttempts int
}

type Worker struct {
	repo   engine.Repository
	relay  *verification.Relay
	config Config
}

func New(repo engine.Repository, relay *verification.Relay, config Config) *Worker {
	if config.ID == "" {
		config.ID = "worker-local"
	}
	if config.Lease <= 0 {
		config.Lease = 30 * time.Second
	}
	if config.Poll <= 0 {
		config.Poll = 500 * time.Millisecond
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 5
	}
	return &Worker{repo: repo, relay: relay, config: config}
}

func (w *Worker) Run(ctx context.Context) error {
	ticker := time.NewTicker(w.config.Poll)
	defer ticker.Stop()
	for {
		processed, err := w.RunOnce(ctx)
		if err != nil {
			return err
		}
		if processed {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *Worker) RunOnce(ctx context.Context) (bool, error) {
	message, ok, err := w.repo.ClaimOutbox(ctx, w.config.ID, w.config.Lease, []string{"verification.run.queued"})
	if err != nil || !ok {
		return false, err
	}
	processErr := w.withHeartbeat(ctx, message, func(processCtx context.Context) error {
		return w.process(processCtx, message)
	})
	if processErr == nil {
		return true, w.repo.CompleteOutbox(ctx, message.ID, w.config.ID)
	}
	if message.Attempts >= w.config.MaxAttempts {
		return true, w.repo.FailOutbox(ctx, message.ID, w.config.ID, "verification_failed")
	}
	delay := retryDelay(message.Attempts)
	return true, w.repo.RetryOutbox(ctx, message.ID, w.config.ID, delay, "verification_retry")
}

func (w *Worker) withHeartbeat(ctx context.Context, message engine.OutboxMessage, operation func(context.Context) error) error {
	processCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	heartbeatErr := make(chan error, 1)
	go func() {
		interval := w.config.Lease / 3
		if interval < 10*time.Millisecond {
			interval = 10 * time.Millisecond
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-processCtx.Done():
				heartbeatErr <- nil
				return
			case <-ticker.C:
				alive, err := w.repo.HeartbeatOutbox(processCtx, message.ID, w.config.ID, w.config.Lease)
				if err != nil || !alive {
					if err == nil {
						err = errors.New("outbox lease lost")
					}
					cancel()
					heartbeatErr <- err
					return
				}
			}
		}
	}()
	operationErr := operation(processCtx)
	cancel()
	if heartbeat := <-heartbeatErr; heartbeat != nil {
		return heartbeat
	}
	return operationErr
}

func (w *Worker) process(ctx context.Context, message engine.OutboxMessage) error {
	if message.Topic != "verification.run.queued" {
		return nil
	}
	var payload struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(message.Payload, &payload); err != nil || payload.RunID == "" {
		return errors.New("invalid verification job payload")
	}
	run, ok, err := w.repo.Run(ctx, payload.RunID)
	if err != nil || !ok {
		return errors.New("verification run unavailable")
	}
	payloadHash := sha256.Sum256([]byte(run.ID + "\n" + run.StripeObjectID + "\n" + run.Environment))
	result, err := w.relay.Execute(ctx, verification.Delivery{
		EventID: "verify_" + run.ID, EffectID: "merchant_effect_" + run.ID,
		RunID: run.ID, Sequence: 1, PayloadHash: hex.EncodeToString(payloadHash[:]),
	}, verification.FaultDuplicate)
	if err != nil {
		return err
	}
	passed := result.BusinessEffects == 1 && result.Duplicates >= 1 && result.Rejected == 0
	return w.repo.RecordVerification(ctx, engine.VerificationEvidence{
		RunID: run.ID, Checkpoint: "reference-merchant-v1",
		Assertion: domain.Assertion{
			ID: "assert_reference_merchant_exactly_once", Name: "Duplicate delivery creates exactly one reference merchant effect",
			Passed: passed, Expected: "1 business effect", Observed: fmt.Sprintf("%d business effect(s), %d duplicate(s)", result.BusinessEffects, result.Duplicates),
			Evidence: verification.ContractVersion,
		},
	})
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 6 {
		attempt = 6
	}
	return time.Duration(1<<(attempt-1)) * time.Second
}
