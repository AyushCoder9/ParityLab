package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/ayushkumarsingh/paritylab/services/api/internal/domain"
)

var demoEpoch = time.Date(2026, time.July, 18, 9, 30, 0, 0, time.UTC)

type idempotencyRecord struct {
	hash string
	run  domain.Run
}

type Service struct {
	mu          sync.RWMutex
	scenarios   []domain.Scenario
	runs        map[string]domain.Run
	events      map[string][]domain.Event
	reports     map[string]domain.Report
	idempotency map[string]idempotencyRecord
	webhooks    map[string]struct{}
	nextRun     int
}

func NewService() *Service {
	s := &Service{
		scenarios:   seededScenarios(),
		runs:        make(map[string]domain.Run),
		events:      make(map[string][]domain.Event),
		reports:     make(map[string]domain.Report),
		idempotency: make(map[string]idempotencyRecord),
		webhooks:    make(map[string]struct{}),
		nextRun:     1,
	}
	s.seedRuns()
	return s
}

func (s *Service) Scenarios() []domain.Scenario {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]domain.Scenario(nil), s.scenarios...)
}

func (s *Service) Scenario(id string) (domain.Scenario, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
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
	hashBytes := sha256.Sum256(requestBody)
	requestHash := hex.EncodeToString(hashBytes[:])

	s.mu.Lock()
	defer s.mu.Unlock()

	if previous, exists := s.idempotency[idempotencyKey]; exists {
		if previous.hash != requestHash {
			return domain.Run{}, false, domain.Conflict("idempotency_key_in_use", "This idempotency key was already used with different request parameters.", "Idempotency-Key")
		}
		return previous.run, true, nil
	}

	var scenario domain.Scenario
	found := false
	for _, item := range s.scenarios {
		if item.ID == scenarioID {
			scenario = item
			found = true
			break
		}
	}
	if !found {
		return domain.Run{}, false, domain.NotFound("scenario", scenarioID)
	}
	if fault == "" {
		fault = domain.FaultNone
	}
	if !supportsFault(scenario, fault) {
		return domain.Run{}, false, domain.Invalid("fault_not_supported", fmt.Sprintf("Scenario %q does not support fault %q.", scenarioID, fault), "fault")
	}

	run := s.buildRunLocked(scenario, fault, demoEpoch.Add(time.Duration(s.nextRun)*23*time.Minute))
	s.idempotency[idempotencyKey] = idempotencyRecord{hash: requestHash, run: run}
	return run, false, nil
}

func (s *Service) Run(id string) (domain.Run, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.runs[id]
	return run, ok
}

func (s *Service) Events(id string) ([]domain.Event, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	events, ok := s.events[id]
	return append([]domain.Event(nil), events...), ok
}

func (s *Service) Report(id string) (domain.Report, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	report, ok := s.reports[id]
	return report, ok
}

func (s *Service) MarkWebhookSeen(eventID string) (duplicate bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.webhooks[eventID]; exists {
		return true
	}
	s.webhooks[eventID] = struct{}{}
	return false
}

func (s *Service) Overview() domain.Overview {
	s.mu.RLock()
	defer s.mu.RUnlock()

	runs := make([]domain.Run, 0, len(s.runs))
	stats := domain.OverviewStats{}
	for _, run := range s.runs {
		runs = append(runs, run)
		stats.TotalRuns++
		stats.EventsProcessed += run.EventCount
		if run.Status == domain.RunPassed {
			stats.PassedRuns++
		}
		for _, event := range s.events[run.ID] {
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
		ReadinessScore: 96,
		Grade:          "production_ready",
		Environment:    "sandbox",
		LastVerifiedAt: lastVerified,
		Stats:          stats,
		Categories: []domain.CategoryReadiness{
			{ID: "idempotency", Label: "Idempotency", Score: 100},
			{ID: "webhooks", Label: "Webhook resilience", Score: 98},
			{ID: "subscriptions", Label: "Subscription lifecycle", Score: 94},
			{ID: "reconciliation", Label: "State convergence", Score: 93},
		},
		RecentRuns: runs,
	}
}

func (s *Service) seedRuns() {
	s.mu.Lock()
	defer s.mu.Unlock()
	seeds := []struct {
		scenario string
		fault    domain.Fault
	}{
		{"checkout-duplicate", domain.FaultDuplicate},
		{"webhook-disorder", domain.FaultReorder},
		{"endpoint-recovery", domain.FaultTimeout},
	}
	for i, seed := range seeds {
		for _, scenario := range s.scenarios {
			if scenario.ID == seed.scenario {
				s.buildRunLocked(scenario, seed.fault, demoEpoch.Add(time.Duration(i)*17*time.Minute))
				break
			}
		}
	}
}

func (s *Service) buildRunLocked(scenario domain.Scenario, fault domain.Fault, started time.Time) domain.Run {
	number := s.nextRun
	s.nextRun++
	id := fmt.Sprintf("run_%06d", number)
	events := buildEvents(id, fault, started)
	duration := events[len(events)-1].At.Sub(events[0].At)
	run := domain.Run{
		ID:              id,
		ScenarioID:      scenario.ID,
		ScenarioName:    scenario.Name,
		Fault:           fault,
		Status:          domain.RunPassed,
		Score:           scoreForFault(fault),
		StartedAt:       started,
		CompletedAt:     started.Add(duration),
		DurationMS:      int(duration.Milliseconds()),
		EventCount:      len(events),
		FindingCount:    findingCount(fault),
		Recovered:       fault != domain.FaultTamper,
		Environment:     "sandbox",
		StripeObjectID:  fmt.Sprintf("pi_demo_%06d", number),
		MerchantOrderID: fmt.Sprintf("ord_%06d", number),
	}
	if fault == domain.FaultTamper {
		run.Status = domain.RunFailed
	}
	s.runs[id] = run
	s.events[id] = events
	s.reports[id] = buildReport(run, fault)
	return run
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
