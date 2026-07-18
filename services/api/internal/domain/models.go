package domain

import "time"

type Fault string

const (
	FaultNone      Fault = "none"
	FaultDuplicate Fault = "duplicate"
	FaultReorder   Fault = "reorder"
	FaultTimeout   Fault = "timeout"
	FaultTamper    Fault = "tamper"
)

type Scenario struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	Category            string   `json:"category"`
	DurationMS          int      `json:"duration_ms"`
	SupportedFaults     []Fault  `json:"supported_faults"`
	Assertions          []string `json:"assertions"`
	Difficulty          string   `json:"difficulty"`
	Recommended         bool     `json:"recommended"`
	EstimatedEventCount int      `json:"estimated_event_count"`
}

type RunStatus string

const (
	RunRunning RunStatus = "running"
	RunPassed  RunStatus = "passed"
	RunFailed  RunStatus = "failed"
)

type Run struct {
	ID              string    `json:"id"`
	ScenarioID      string    `json:"scenario_id"`
	ScenarioName    string    `json:"scenario_name"`
	Fault           Fault     `json:"fault"`
	Status          RunStatus `json:"status"`
	Score           int       `json:"score"`
	StartedAt       time.Time `json:"started_at"`
	CompletedAt     time.Time `json:"completed_at"`
	DurationMS      int       `json:"duration_ms"`
	EventCount      int       `json:"event_count"`
	FindingCount    int       `json:"finding_count"`
	Recovered       bool      `json:"recovered"`
	Environment     string    `json:"environment"`
	StripeObjectID  string    `json:"stripe_object_id"`
	MerchantOrderID string    `json:"merchant_order_id"`
}

type EventStatus string

const (
	EventHealthy   EventStatus = "healthy"
	EventDiverged  EventStatus = "diverged"
	EventRecovered EventStatus = "recovered"
	EventBlocked   EventStatus = "blocked"
)

type Event struct {
	ID          string         `json:"id"`
	RunID       string         `json:"run_id"`
	Sequence    int            `json:"sequence"`
	At          time.Time      `json:"at"`
	Source      string         `json:"source"`
	Target      string         `json:"target"`
	Type        string         `json:"type"`
	Title       string         `json:"title"`
	Detail      string         `json:"detail"`
	Status      EventStatus    `json:"status"`
	LatencyMS   int            `json:"latency_ms"`
	Checkpoint  string         `json:"checkpoint"`
	TraceID     string         `json:"trace_id"`
	Evidence    map[string]any `json:"evidence,omitempty"`
	IsDuplicate bool           `json:"is_duplicate,omitempty"`
}

type Assertion struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Passed   bool   `json:"passed"`
	Expected string `json:"expected"`
	Observed string `json:"observed"`
	Evidence string `json:"evidence"`
}

type Finding struct {
	ID          string `json:"id"`
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	Summary     string `json:"summary"`
	Cause       string `json:"cause"`
	Remediation string `json:"remediation"`
	Checkpoint  string `json:"checkpoint"`
	Resolved    bool   `json:"resolved"`
}

type StateSnapshot struct {
	Stripe   string `json:"stripe"`
	Webhook  string `json:"webhook"`
	Merchant string `json:"merchant"`
	Balanced bool   `json:"balanced"`
}

type Report struct {
	Run        Run           `json:"run"`
	Summary    string        `json:"summary"`
	Verdict    string        `json:"verdict"`
	Assertions []Assertion   `json:"assertions"`
	Findings   []Finding     `json:"findings"`
	State      StateSnapshot `json:"state"`
	Generated  time.Time     `json:"generated_at"`
}

type Overview struct {
	ReadinessScore int                 `json:"readiness_score"`
	Grade          string              `json:"grade"`
	Environment    string              `json:"environment"`
	LastVerifiedAt time.Time           `json:"last_verified_at"`
	Stats          OverviewStats       `json:"stats"`
	Categories     []CategoryReadiness `json:"categories"`
	RecentRuns     []Run               `json:"recent_runs"`
	Critical       *Finding            `json:"critical_finding"`
}

type OverviewStats struct {
	TotalRuns        int `json:"total_runs"`
	PassedRuns       int `json:"passed_runs"`
	EventsProcessed  int `json:"events_processed"`
	DuplicatesCaught int `json:"duplicates_caught"`
	P95LatencyMS     int `json:"p95_latency_ms"`
}

type CategoryReadiness struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Score int    `json:"score"`
}
