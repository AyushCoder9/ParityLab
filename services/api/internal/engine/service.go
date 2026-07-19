package engine

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/domain"
)

var demoEpoch = time.Date(2026, time.July, 18, 9, 30, 0, 0, time.UTC)

type Service struct {
	scenarios []domain.Scenario
	repo      Repository
}

// NewService retains the credential-free deterministic behavior used by local
// development and unit tests.
func NewService() *Service {
	service, err := NewServiceWithRepository(NewMemoryRepository())
	if err != nil {
		panic(err)
	}
	return service
}

func NewServiceWithRepository(repo Repository) (*Service, error) {
	if repo == nil {
		return nil, errors.New("repository is required")
	}
	service := &Service{scenarios: seededScenarios(), repo: repo}
	if err := service.ensureSeedRuns(context.Background()); err != nil {
		return nil, fmt.Errorf("seed deterministic runs: %w", err)
	}
	return service, nil
}

func (s *Service) Close() error { return s.repo.Close() }

func (s *Service) Scenarios() []domain.Scenario {
	return append([]domain.Scenario(nil), s.scenarios...)
}

func (s *Service) Scenario(id string) (domain.Scenario, bool) {
	for _, scenario := range s.scenarios {
		if scenario.ID == id {
			return scenario, true
		}
	}
	return domain.Scenario{}, false
}

func (s *Service) CreateRun(scenarioID string, fault domain.Fault, idempotencyKey string, requestBody []byte) (domain.Run, bool, *domain.Error) {
	if idempotencyKey == "" {
		return domain.Run{}, false, domain.Invalid("idempotency_key_missing", "An Idempotency-Key header is required for this request.", "Idempotency-Key")
	}
	scenario, found := s.Scenario(scenarioID)
	if !found {
		return domain.Run{}, false, domain.NotFound("scenario", scenarioID)
	}
	if fault == "" {
		fault = domain.FaultNone
	}
	if !supportsFault(scenario, fault) {
		return domain.Run{}, false, domain.Invalid("fault_not_supported", fmt.Sprintf("Scenario %q does not support fault %q.", scenarioID, fault), "fault")
	}

	ctx := context.Background()
	keyHash := sha256.Sum256([]byte(idempotencyKey))
	requestHash := sha256.Sum256(requestBody)
	if replay, found, err := s.repo.ReplayRun(ctx, keyHash, requestHash); err != nil {
		if errors.Is(err, ErrIdempotencyConflict) {
			return domain.Run{}, false, domain.Conflict("idempotency_key_in_use", "This idempotency key was already used with different request parameters.", "Idempotency-Key")
		}
		return domain.Run{}, false, domain.Internal("persistence_failed", "The idempotency record could not be read.")
	} else if found {
		return replay, true, nil
	}
	id, err := s.repo.NextRunID(ctx)
	if err != nil {
		return domain.Run{}, false, domain.Internal("persistence_unavailable", "The run could not be durably allocated.")
	}
	number := numberFromRunID(id)
	bundle := buildRunBundle(id, number, scenario, fault, demoEpoch.Add(time.Duration(number)*23*time.Minute))
	run, replayed, err := s.repo.CreateRun(ctx, keyHash, requestHash, bundle)
	if errors.Is(err, ErrIdempotencyConflict) {
		return domain.Run{}, false, domain.Conflict("idempotency_key_in_use", "This idempotency key was already used with different request parameters.", "Idempotency-Key")
	}
	if err != nil {
		return domain.Run{}, false, domain.Internal("persistence_failed", "The run could not be durably persisted.")
	}
	return run, replayed, nil
}

func (s *Service) Run(id string) (domain.Run, bool) {
	run, ok, err := s.repo.Run(context.Background(), id)
	return run, ok && err == nil
}

func (s *Service) Runs() []domain.Run {
	runs, err := s.repo.ListRuns(context.Background())
	if err != nil {
		return []domain.Run{}
	}
	return runs
}

func (s *Service) Events(id string) ([]domain.Event, bool) {
	events, ok, err := s.repo.Events(context.Background(), id)
	return events, ok && err == nil
}

func (s *Service) Report(id string) (domain.Report, bool) {
	report, ok, err := s.repo.Report(context.Background(), id)
	return report, ok && err == nil
}

// RecordWebhook stores hashes and non-sensitive envelope metadata only.
func (s *Service) RecordWebhook(eventID, eventType, endpointToken string, body []byte) (bool, error) {
	receipt := WebhookReceipt{
		EventID: eventID, EventType: eventType,
		EndpointTokenSHA: sha256.Sum256([]byte(endpointToken)),
		BodySHA:          sha256.Sum256(body),
	}
	return s.repo.MarkWebhookSeen(context.Background(), receipt)
}

// MarkWebhookSeen remains for existing engine consumers and tests.
func (s *Service) MarkWebhookSeen(eventID string) (duplicate bool) {
	duplicate, err := s.RecordWebhook(eventID, "unknown", "", nil)
	return duplicate || err != nil
}

func (s *Service) Overview() domain.Overview {
	runs, err := s.repo.ListRuns(context.Background())
	if err != nil {
		runs = nil
	}
	stats := domain.OverviewStats{}
	for _, run := range runs {
		stats.TotalRuns++
		stats.EventsProcessed += run.EventCount
		if run.Status == domain.RunPassed {
			stats.PassedRuns++
		}
		events, _, _ := s.repo.Events(context.Background(), run.ID)
		for _, event := range events {
			if event.IsDuplicate {
				stats.DuplicatesCaught++
			}
		}
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].StartedAt.After(runs[j].StartedAt) })
	if len(runs) > 5 {
		runs = runs[:5]
	}
	stats.P95LatencyMS = 184
	lastVerified := demoEpoch
	if len(runs) > 0 {
		lastVerified = runs[0].CompletedAt
	}
	return domain.Overview{
		ReadinessScore: 96, Grade: "production_ready", Environment: "sandbox", LastVerifiedAt: lastVerified, Stats: stats,
		Categories: []domain.CategoryReadiness{
			{ID: "idempotency", Label: "Idempotency", Score: 100},
			{ID: "webhooks", Label: "Webhook resilience", Score: 98},
			{ID: "subscriptions", Label: "Subscription lifecycle", Score: 94},
			{ID: "reconciliation", Label: "State convergence", Score: 93},
		}, RecentRuns: runs,
	}
}

func (s *Service) ensureSeedRuns(ctx context.Context) error {
	seeds := []struct {
		scenario string
		fault    domain.Fault
	}{
		{"checkout-duplicate", domain.FaultDuplicate},
		{"webhook-disorder", domain.FaultReorder},
		{"endpoint-recovery", domain.FaultTimeout},
	}
	for index, seed := range seeds {
		id := runID(index + 1)
		if _, exists, err := s.repo.Run(ctx, id); err != nil {
			return err
		} else if exists {
			continue
		}
		scenario, _ := s.Scenario(seed.scenario)
		number := numberFromRunID(id)
		bundle := buildRunBundle(id, number, scenario, seed.fault, demoEpoch.Add(time.Duration(index)*17*time.Minute))
		keyHash := sha256.Sum256([]byte("paritylab:seed:" + seed.scenario))
		requestHash := sha256.Sum256([]byte(seed.scenario + ":" + string(seed.fault)))
		if _, _, err := s.repo.CreateRun(ctx, keyHash, requestHash, bundle); err != nil {
			return err
		}
	}
	return nil
}

func buildRunBundle(id string, number int, scenario domain.Scenario, fault domain.Fault, started time.Time) RunBundle {
	events := buildEvents(id, fault, started)
	duration := events[len(events)-1].At.Sub(events[0].At)
	run := domain.Run{
		ID: id, ScenarioID: scenario.ID, ScenarioName: scenario.Name, Fault: fault,
		Status: domain.RunPassed, Score: scoreForFault(fault), StartedAt: started,
		CompletedAt: started.Add(duration), DurationMS: int(duration.Milliseconds()),
		EventCount: len(events), FindingCount: findingCount(fault), Recovered: fault != domain.FaultTamper,
		Environment: "sandbox", StripeObjectID: fmt.Sprintf("pi_demo_%06d", number),
		MerchantOrderID: fmt.Sprintf("ord_%06d", number),
	}
	if fault == domain.FaultTamper {
		run.Status = domain.RunFailed
	}
	return RunBundle{Run: run, Events: events, Report: buildReport(run, fault)}
}

func runID(number int) string { return fmt.Sprintf("run_%06d", number) }

func numberFromRunID(id string) int {
	value, err := strconv.Atoi(strings.TrimPrefix(id, "run_"))
	if err != nil {
		return 0
	}
	return value
}

func supportsFault(scenario domain.Scenario, fault domain.Fault) bool {
	for _, candidate := range scenario.SupportedFaults {
		if candidate == fault {
			return true
		}
	}
	return false
}

func scoreForFault(fault domain.Fault) int {
	if fault == domain.FaultTamper {
		return 72
	}
	if fault == domain.FaultNone {
		return 100
	}
	return 96
}

func findingCount(fault domain.Fault) int {
	if fault == domain.FaultNone {
		return 0
	}
	return 1
}
