package monitoring

import (
	"testing"
	"time"
)

func TestMySQLRuleEngine_BasicBreach(t *testing.T) {
	rules := []AlertRule{
		{
			ID: "test-rule", Name: "Test Rule", Enabled: true,
			RuleCode: "CONN_UTIL_WARN", Severity: "warning",
			ThresholdNum: 70, ConsecutiveBreaches: 1, RecoverySamples: 1,
		},
	}

	dir := t.TempDir()
	engine, err := NewMySQLRuleEngine(rules, dir)
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}

	// 80% utilization should breach at threshold 70
	sample := MySQLSample{Connections: 80, MaxConnections: 100}

	results := engine.Evaluate("mysql-1", sample, nil)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Action != "open" {
		t.Errorf("expected action 'open', got %q", results[0].Action)
	}
	if results[0].RuleCode != "CONN_UTIL_WARN" {
		t.Errorf("expected rule code CONN_UTIL_WARN, got %s", results[0].RuleCode)
	}
}

func TestMySQLRuleEngine_ConsecutiveBreachStreak(t *testing.T) {
	rules := []AlertRule{
		{
			ID: "streak-rule", Name: "Streak", Enabled: true,
			RuleCode: "CONN_UTIL_WARN", Severity: "warning",
			ThresholdNum: 70, ConsecutiveBreaches: 3, RecoverySamples: 1,
		},
	}

	dir := t.TempDir()
	engine, err := NewMySQLRuleEngine(rules, dir)
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}

	sample := MySQLSample{Connections: 80, MaxConnections: 100}

	// First two breaches shouldn't trigger
	for i := 0; i < 2; i++ {
		results := engine.Evaluate("mysql-1", sample, nil)
		if len(results) != 0 {
			t.Errorf("breach %d: expected no results, got %d", i+1, len(results))
		}
	}

	// Third consecutive breach should trigger
	results := engine.Evaluate("mysql-1", sample, nil)
	if len(results) != 1 || results[0].Action != "open" {
		t.Fatalf("expected open action on 3rd breach, got %+v", results)
	}
}

func TestMySQLRuleEngine_RecoveryStreak(t *testing.T) {
	rules := []AlertRule{
		{
			ID: "recovery-rule", Name: "Recovery", Enabled: true,
			RuleCode: "CONN_UTIL_WARN", Severity: "warning",
			ThresholdNum: 70, ConsecutiveBreaches: 1, RecoverySamples: 3,
		},
	}

	dir := t.TempDir()
	engine, err := NewMySQLRuleEngine(rules, dir)
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}

	// Trigger the rule
	breached := MySQLSample{Connections: 80, MaxConnections: 100}
	results := engine.Evaluate("mysql-1", breached, nil)
	if len(results) != 1 || results[0].Action != "open" {
		t.Fatal("expected open")
	}

	engine.SetOpenIncidentID("CONN_UTIL_WARN", "mysql-1", "incident-1")

	// First two recoveries shouldn't close
	ok := MySQLSample{Connections: 50, MaxConnections: 100}
	for i := 0; i < 2; i++ {
		results := engine.Evaluate("mysql-1", ok, nil)
		if len(results) != 0 {
			t.Errorf("recovery %d: expected no results, got %d", i+1, len(results))
		}
	}

	// Third recovery should close
	results = engine.Evaluate("mysql-1", ok, nil)
	if len(results) != 1 || results[0].Action != "close" {
		t.Fatalf("expected close on 3rd recovery, got %+v", results)
	}
	if results[0].IncidentID != "incident-1" {
		t.Errorf("expected incidentID incident-1, got %s", results[0].IncidentID)
	}
}

func TestMySQLRuleEngine_BreachResetsRecoveryStreak(t *testing.T) {
	rules := []AlertRule{
		{
			ID: "reset-test", Name: "Reset", Enabled: true,
			RuleCode: "CONN_UTIL_WARN", Severity: "warning",
			ThresholdNum: 70, ConsecutiveBreaches: 1, RecoverySamples: 3,
		},
	}

	dir := t.TempDir()
	engine, err := NewMySQLRuleEngine(rules, dir)
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}

	// Open the alert
	engine.Evaluate("mysql-1", MySQLSample{Connections: 80, MaxConnections: 100}, nil)

	// Two OK samples
	ok := MySQLSample{Connections: 50, MaxConnections: 100}
	engine.Evaluate("mysql-1", ok, nil)
	engine.Evaluate("mysql-1", ok, nil)

	// Breach again — should reset recovery streak
	engine.Evaluate("mysql-1", MySQLSample{Connections: 80, MaxConnections: 100}, nil)

	// Need 3 more OK samples to close
	for i := 0; i < 2; i++ {
		results := engine.Evaluate("mysql-1", ok, nil)
		if len(results) != 0 {
			t.Errorf("should not close yet on iteration %d", i)
		}
	}
	results := engine.Evaluate("mysql-1", ok, nil)
	if len(results) != 1 || results[0].Action != "close" {
		t.Fatal("expected close after 3 consecutive OK after reset")
	}
}

func TestMySQLRuleEngine_DisabledRule(t *testing.T) {
	rules := []AlertRule{
		{
			ID: "disabled", Name: "Disabled", Enabled: false,
			RuleCode: "CONN_UTIL_WARN", Severity: "warning",
			ThresholdNum: 70, ConsecutiveBreaches: 1, RecoverySamples: 1,
		},
	}

	dir := t.TempDir()
	engine, err := NewMySQLRuleEngine(rules, dir)
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}

	sample := MySQLSample{Connections: 100, MaxConnections: 100}
	results := engine.Evaluate("mysql-1", sample, nil)
	if len(results) != 0 {
		t.Error("disabled rule should not produce results")
	}
}

func TestMySQLRuleEngine_CheckIDFiltering(t *testing.T) {
	rules := []AlertRule{
		{
			ID: "filtered", Name: "Filtered", Enabled: true,
			RuleCode: "CONN_UTIL_WARN", Severity: "warning",
			ThresholdNum: 70, ConsecutiveBreaches: 1, RecoverySamples: 1,
			CheckIDs: []string{"mysql-2"},
		},
	}

	dir := t.TempDir()
	engine, err := NewMySQLRuleEngine(rules, dir)
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}

	sample := MySQLSample{Connections: 100, MaxConnections: 100}

	// Should not apply to mysql-1
	results := engine.Evaluate("mysql-1", sample, nil)
	if len(results) != 0 {
		t.Error("rule should not apply to mysql-1")
	}

	// Should apply to mysql-2
	results = engine.Evaluate("mysql-2", sample, nil)
	if len(results) != 1 {
		t.Error("rule should apply to mysql-2")
	}
}

func TestMySQLRuleEngine_AllRuleCodes(t *testing.T) {
	now := time.Now().UTC()
	sample := MySQLSample{
		Timestamp:      now,
		Connections:    95,
		MaxConnections: 100,
		ThreadsRunning: 60,
	}
	delta := &MySQLDelta{
		Timestamp:               now,
		ConnectionsRefusedDelta: 5,
		AbortedConnectsPerSec:   10,
		RowLockWaitsPerSec:      15,
		SlowQueriesPerSec:       5,
		TmpDiskTablesPct:        30,
		ThreadsCreatedPerSec:    15,
	}

	tests := []struct {
		ruleCode  string
		threshold float64
		expect    bool
	}{
		{"CONN_UTIL_WARN", 70, true},
		{"CONN_UTIL_CRIT", 90, true},
		{"MAX_CONN_REFUSED", 0, true},
		{"ABORTED_CONNECT_SPIKE", 5, true},
		{"THREADS_RUNNING_HIGH", 50, true},
		{"ROW_LOCK_WAITS_HIGH", 10, true},
		{"SLOW_QUERY_SPIKE", 2, true},
		{"TMP_DISK_PCT_HIGH", 25, true},
		{"THREAD_CREATE_SPIKE", 10, true},
	}

	for _, tt := range tests {
		t.Run(tt.ruleCode, func(t *testing.T) {
			rules := []AlertRule{
				{
					ID: tt.ruleCode, Name: tt.ruleCode, Enabled: true,
					RuleCode: tt.ruleCode, Severity: "warning",
					ThresholdNum: tt.threshold, ConsecutiveBreaches: 1, RecoverySamples: 1,
				},
			}

			dir := t.TempDir()
			engine, err := NewMySQLRuleEngine(rules, dir)
			if err != nil {
				t.Fatalf("create engine: %v", err)
			}

			results := engine.Evaluate("test", sample, delta)
			if tt.expect && len(results) == 0 {
				t.Error("expected breach but got none")
			}
			if !tt.expect && len(results) > 0 {
				t.Errorf("expected no breach but got %d", len(results))
			}
		})
	}
}

func TestMySQLRuleEngine_NilDeltaDoesNotBreachDeltaRules(t *testing.T) {
	deltaOnlyRules := []string{
		"MAX_CONN_REFUSED", "ABORTED_CONNECT_SPIKE",
		"ROW_LOCK_WAITS_HIGH", "SLOW_QUERY_SPIKE",
		"TMP_DISK_PCT_HIGH", "THREAD_CREATE_SPIKE",
	}

	for _, code := range deltaOnlyRules {
		t.Run(code, func(t *testing.T) {
			rules := []AlertRule{
				{
					ID: code, Name: code, Enabled: true,
					RuleCode: code, Severity: "warning",
					ThresholdNum: 0, ConsecutiveBreaches: 1, RecoverySamples: 1,
				},
			}

			dir := t.TempDir()
			engine, err := NewMySQLRuleEngine(rules, dir)
			if err != nil {
				t.Fatalf("create engine: %v", err)
			}

			results := engine.Evaluate("test", MySQLSample{}, nil)
			if len(results) != 0 {
				t.Errorf("rule %s should not breach with nil delta", code)
			}
		})
	}
}

func TestMySQLRuleEngine_ZeroMaxConnections(t *testing.T) {
	rules := []AlertRule{
		{
			ID: "zero-max", Name: "Zero Max", Enabled: true,
			RuleCode: "CONN_UTIL_WARN", Severity: "warning",
			ThresholdNum: 70, ConsecutiveBreaches: 1, RecoverySamples: 1,
		},
	}

	dir := t.TempDir()
	engine, err := NewMySQLRuleEngine(rules, dir)
	if err != nil {
		t.Fatalf("create engine: %v", err)
	}

	// MaxConnections=0 should not panic/breach
	sample := MySQLSample{Connections: 100, MaxConnections: 0}
	results := engine.Evaluate("test", sample, nil)
	if len(results) != 0 {
		t.Error("should not breach with MaxConnections=0")
	}
}

func TestMySQLRuleEngine_StatePersistence(t *testing.T) {
	dir := t.TempDir()
	rules := []AlertRule{
		{
			ID: "persist", Name: "Persist", Enabled: true,
			RuleCode: "CONN_UTIL_WARN", Severity: "warning",
			ThresholdNum: 70, ConsecutiveBreaches: 3, RecoverySamples: 1,
		},
	}

	// Create engine and accumulate 2 breaches
	engine1, _ := NewMySQLRuleEngine(rules, dir)
	sample := MySQLSample{Connections: 80, MaxConnections: 100}
	engine1.Evaluate("mysql-1", sample, nil) // breach 1
	engine1.Evaluate("mysql-1", sample, nil) // breach 2

	// Reload from disk — state should survive
	engine2, err := NewMySQLRuleEngine(rules, dir)
	if err != nil {
		t.Fatalf("reload engine: %v", err)
	}

	// Third breach should now trigger
	results := engine2.Evaluate("mysql-1", sample, nil)
	if len(results) != 1 || results[0].Action != "open" {
		t.Fatal("expected open on 3rd breach after state reload")
	}
}

func TestDefaultMySQLRules(t *testing.T) {
	rules := DefaultMySQLRules()
	if len(rules) != 9 {
		t.Errorf("expected 9 default rules, got %d", len(rules))
	}

	// Verify all are enabled
	for _, r := range rules {
		if !r.Enabled {
			t.Errorf("default rule %s should be enabled", r.RuleCode)
		}
		if r.ConsecutiveBreaches <= 0 {
			t.Errorf("rule %s should have positive consecutive breaches", r.RuleCode)
		}
		if r.RecoverySamples <= 0 {
			t.Errorf("rule %s should have positive recovery samples", r.RuleCode)
		}
	}
}

func TestRuleAppliesToCheck(t *testing.T) {
	tests := []struct {
		name     string
		checkIDs []string
		checkID  string
		want     bool
	}{
		{"empty applies to all", nil, "any-check", true},
		{"in list", []string{"a", "b"}, "a", true},
		{"not in list", []string{"a", "b"}, "c", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := AlertRule{CheckIDs: tt.checkIDs}
			got := ruleAppliesToCheck(rule, tt.checkID)
			if got != tt.want {
				t.Errorf("ruleAppliesToCheck = %v, want %v", got, tt.want)
			}
		})
	}
}
