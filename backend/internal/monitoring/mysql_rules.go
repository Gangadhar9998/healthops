package monitoring

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// MySQLRuleEngine evaluates MySQL-specific rules with consecutive breach and recovery streak logic.
type MySQLRuleEngine struct {
	mu     sync.Mutex
	rules  []AlertRule
	states map[string]*AlertState // key: ruleCode+checkID
	path   string
}

// NewMySQLRuleEngine creates a new rule engine with the given rules and state persistence path.
func NewMySQLRuleEngine(rules []AlertRule, dataDir string) (*MySQLRuleEngine, error) {
	statePath := filepath.Join(dataDir, "mysql_rule_states.json")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create rule state dir: %w", err)
	}

	engine := &MySQLRuleEngine{
		rules:  rules,
		states: make(map[string]*AlertState),
		path:   statePath,
	}

	// Load persisted state
	if raw, err := os.ReadFile(statePath); err == nil {
		var states map[string]*AlertState
		if err := json.Unmarshal(raw, &states); err == nil {
			engine.states = states
		}
	}

	return engine, nil
}

// stateKey returns the unique key for a rule+check pair.
func stateKey(ruleCode, checkID string) string {
	return ruleCode + ":" + checkID
}

// EvaluateResult represents the outcome of evaluating a sample against rules.
type EvaluateResult struct {
	RuleCode   string
	CheckID    string
	Action     string // "open", "close", or ""
	Severity   string
	Message    string
	IncidentID string // populated on close
}

// Evaluate processes a sample and its delta against all rules for a given check.
func (e *MySQLRuleEngine) Evaluate(checkID string, sample MySQLSample, delta *MySQLDelta) []EvaluateResult {
	e.mu.Lock()
	defer e.mu.Unlock()

	var results []EvaluateResult

	for _, rule := range e.rules {
		if !rule.Enabled {
			continue
		}
		if !ruleAppliesToCheck(rule, checkID) {
			continue
		}

		breached := e.isBreached(rule, sample, delta)
		key := stateKey(rule.RuleCode, checkID)
		state, exists := e.states[key]
		if !exists {
			state = &AlertState{
				RuleCode: rule.RuleCode,
				CheckID:  checkID,
				Status:   "OK",
			}
			e.states[key] = state
		}

		result := e.transition(state, rule, breached)
		if result.Action != "" {
			results = append(results, result)
		}
	}

	// Persist state after evaluation
	_ = e.persistStates()

	return results
}

// transition handles state transitions for a rule+check pair.
func (e *MySQLRuleEngine) transition(state *AlertState, rule AlertRule, breached bool) EvaluateResult {
	consecutiveBreaches := rule.ConsecutiveBreaches
	if consecutiveBreaches <= 0 {
		consecutiveBreaches = 1
	}
	recoverySamples := rule.RecoverySamples
	if recoverySamples <= 0 {
		recoverySamples = 1
	}

	result := EvaluateResult{
		RuleCode: rule.RuleCode,
		CheckID:  state.CheckID,
		Severity: rule.Severity,
	}

	if breached {
		state.RecoveryStreak = 0
		state.BreachStreak++

		if state.Status == "OK" && state.BreachStreak >= consecutiveBreaches {
			state.Status = "OPEN"
			result.Action = "open"
			result.Message = fmt.Sprintf("Rule %s breached %d consecutive times", rule.RuleCode, state.BreachStreak)
		}
	} else {
		state.BreachStreak = 0
		state.RecoveryStreak++

		if state.Status == "OPEN" && state.RecoveryStreak >= recoverySamples {
			state.Status = "OK"
			state.RecoveryStreak = 0
			result.Action = "close"
			result.IncidentID = state.OpenIncidentID
			result.Message = fmt.Sprintf("Rule %s recovered after %d consecutive OK samples", rule.RuleCode, recoverySamples)
			state.OpenIncidentID = ""
		}
	}

	return result
}

// SetOpenIncidentID records the incident ID for an open rule+check pair.
func (e *MySQLRuleEngine) SetOpenIncidentID(ruleCode, checkID, incidentID string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	key := stateKey(ruleCode, checkID)
	if state, ok := e.states[key]; ok {
		state.OpenIncidentID = incidentID
	}
}

// isBreached evaluates whether a rule is breached given the current sample/delta.
func (e *MySQLRuleEngine) isBreached(rule AlertRule, sample MySQLSample, delta *MySQLDelta) bool {
	switch rule.RuleCode {
	case "CONN_UTIL_WARN":
		if sample.MaxConnections == 0 {
			return false
		}
		utilPct := float64(sample.Connections) / float64(sample.MaxConnections) * 100
		return utilPct >= rule.ThresholdNum

	case "CONN_UTIL_CRIT":
		if sample.MaxConnections == 0 {
			return false
		}
		utilPct := float64(sample.Connections) / float64(sample.MaxConnections) * 100
		return utilPct >= rule.ThresholdNum

	case "MAX_CONN_REFUSED":
		if delta == nil {
			return false
		}
		return delta.ConnectionsRefusedDelta > 0

	case "ABORTED_CONNECT_SPIKE":
		if delta == nil {
			return false
		}
		return delta.AbortedConnectsPerSec >= rule.ThresholdNum

	case "THREADS_RUNNING_HIGH":
		return float64(sample.ThreadsRunning) >= rule.ThresholdNum

	case "ROW_LOCK_WAITS_HIGH":
		if delta == nil {
			return false
		}
		return delta.RowLockWaitsPerSec >= rule.ThresholdNum

	case "SLOW_QUERY_SPIKE":
		if delta == nil {
			return false
		}
		return delta.SlowQueriesPerSec >= rule.ThresholdNum

	case "TMP_DISK_PCT_HIGH":
		if delta == nil {
			return false
		}
		return delta.TmpDiskTablesPct >= rule.ThresholdNum

	case "THREAD_CREATE_SPIKE":
		if delta == nil {
			return false
		}
		return delta.ThreadsCreatedPerSec >= rule.ThresholdNum

	default:
		return false
	}
}

func ruleAppliesToCheck(rule AlertRule, checkID string) bool {
	if len(rule.CheckIDs) == 0 {
		return true
	}
	for _, id := range rule.CheckIDs {
		if id == checkID {
			return true
		}
	}
	return false
}

func (e *MySQLRuleEngine) persistStates() error {
	data, err := json.MarshalIndent(e.states, "", "  ")
	if err != nil {
		return err
	}
	tmp := e.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, e.path)
}

// DefaultMySQLRules returns the v1 default MySQL alert rules with starter thresholds.
func DefaultMySQLRules() []AlertRule {
	return []AlertRule{
		{
			ID: "mysql-conn-util-warn", Name: "Connection Utilization Warning",
			Enabled: true, RuleCode: "CONN_UTIL_WARN", Severity: "warning",
			ThresholdNum: 70, ConsecutiveBreaches: 3, RecoverySamples: 3,
			CooldownMinutes: 15, Description: "Connection utilization above 70%",
		},
		{
			ID: "mysql-conn-util-crit", Name: "Connection Utilization Critical",
			Enabled: true, RuleCode: "CONN_UTIL_CRIT", Severity: "critical",
			ThresholdNum: 90, ConsecutiveBreaches: 2, RecoverySamples: 3,
			CooldownMinutes: 5, Description: "Connection utilization above 90%",
		},
		{
			ID: "mysql-max-conn-refused", Name: "Max Connections Refused",
			Enabled: true, RuleCode: "MAX_CONN_REFUSED", Severity: "critical",
			ThresholdNum: 0, ConsecutiveBreaches: 1, RecoverySamples: 5,
			CooldownMinutes: 5, Description: "Connection refused due to max_connections",
		},
		{
			ID: "mysql-aborted-connect-spike", Name: "Aborted Connect Spike",
			Enabled: true, RuleCode: "ABORTED_CONNECT_SPIKE", Severity: "warning",
			ThresholdNum: 5, ConsecutiveBreaches: 3, RecoverySamples: 5,
			CooldownMinutes: 15, Description: "Aborted connects per second exceeds threshold",
		},
		{
			ID: "mysql-threads-running-high", Name: "Threads Running High",
			Enabled: true, RuleCode: "THREADS_RUNNING_HIGH", Severity: "warning",
			ThresholdNum: 50, ConsecutiveBreaches: 3, RecoverySamples: 3,
			CooldownMinutes: 10, Description: "Threads running exceeds threshold",
		},
		{
			ID: "mysql-row-lock-waits-high", Name: "Row Lock Waits High",
			Enabled: true, RuleCode: "ROW_LOCK_WAITS_HIGH", Severity: "warning",
			ThresholdNum: 10, ConsecutiveBreaches: 3, RecoverySamples: 5,
			CooldownMinutes: 15, Description: "InnoDB row lock waits per second exceeds threshold",
		},
		{
			ID: "mysql-slow-query-spike", Name: "Slow Query Spike",
			Enabled: true, RuleCode: "SLOW_QUERY_SPIKE", Severity: "warning",
			ThresholdNum: 2, ConsecutiveBreaches: 3, RecoverySamples: 5,
			CooldownMinutes: 15, Description: "Slow queries per second exceeds threshold",
		},
		{
			ID: "mysql-tmp-disk-pct-high", Name: "Temp Disk Tables Percentage High",
			Enabled: true, RuleCode: "TMP_DISK_PCT_HIGH", Severity: "warning",
			ThresholdNum: 25, ConsecutiveBreaches: 5, RecoverySamples: 5,
			CooldownMinutes: 30, Description: "Percentage of temp tables on disk exceeds threshold",
		},
		{
			ID: "mysql-thread-create-spike", Name: "Thread Create Spike",
			Enabled: true, RuleCode: "THREAD_CREATE_SPIKE", Severity: "warning",
			ThresholdNum: 10, ConsecutiveBreaches: 3, RecoverySamples: 5,
			CooldownMinutes: 15, Description: "Thread creation rate per second exceeds threshold",
		},
	}
}
