package monitoring

import (
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"
)

// mockLogger is a simple logger implementation for testing.
type mockLogger struct {
	messages []string
}

func (m *mockLogger) Printf(format string, v ...interface{}) {
	msg := fmt.Sprintf(format, v...)
	m.messages = append(m.messages, msg)
}

func TestAlertRuleEngine_Evaluate_StatusCondition(t *testing.T) {
	rules := []AlertRule{
		{
			ID:       "rule-1",
			Name:     "Critical Status Alert",
			Enabled:  true,
			CheckIDs: []string{"check-1"},
			Conditions: []AlertCondition{
				{
					Field:    "status",
					Operator: OperatorEquals,
					Value:    "critical",
				},
			},
			Severity:        "critical",
			Channels:        []AlertChannel{{Type: "log"}},
			CooldownMinutes: 0,
		},
	}

	logger := &mockLogger{}
	engine := NewAlertRuleEngine(rules, logger)

	results := []CheckResult{
		{
			ID:         "result-1",
			CheckID:    "check-1",
			Name:       "Test Check",
			Status:     "critical",
			Healthy:    false,
			DurationMs: 100,
			StartedAt:  time.Now().UTC(),
			FinishedAt: time.Now().UTC(),
			Metrics:    map[string]float64{},
		},
	}

	alerts := engine.Evaluate(results)
	if len(alerts) != 1 {
		t.Errorf("expected 1 alert, got %d", len(alerts))
	}

	if alerts[0].RuleID != "rule-1" {
		t.Errorf("expected rule ID rule-1, got %s", alerts[0].RuleID)
	}

	if alerts[0].Severity != "critical" {
		t.Errorf("expected severity critical, got %s", alerts[0].Severity)
	}
}

func TestAlertRuleEngine_Evaluate_MultipleConditions(t *testing.T) {
	rules := []AlertRule{
		{
			ID:       "rule-1",
			Name:     "Slow Critical Alert",
			Enabled:  true,
			CheckIDs: []string{"check-1"},
			Conditions: []AlertCondition{
				{
					Field:    "status",
					Operator: OperatorEquals,
					Value:    "critical",
				},
				{
					Field:    "durationMs",
					Operator: OperatorGreaterThan,
					Value:    1000,
				},
			},
			Severity:        "critical",
			Channels:        []AlertChannel{{Type: "log"}},
			CooldownMinutes: 0,
		},
	}

	logger := &mockLogger{}
	engine := NewAlertRuleEngine(rules, logger)

	// Should trigger: both conditions match
	results := []CheckResult{
		{
			ID:         "result-1",
			CheckID:    "check-1",
			Name:       "Test Check",
			Status:     "critical",
			Healthy:    false,
			DurationMs: 1500,
			StartedAt:  time.Now().UTC(),
			FinishedAt: time.Now().UTC(),
			Metrics:    map[string]float64{},
		},
	}

	alerts := engine.Evaluate(results)
	if len(alerts) != 1 {
		t.Errorf("expected 1 alert, got %d", len(alerts))
	}

	// Should NOT trigger: second condition doesn't match
	results2 := []CheckResult{
		{
			ID:         "result-2",
			CheckID:    "check-1",
			Name:       "Test Check",
			Status:     "critical",
			Healthy:    false,
			DurationMs: 500,
			StartedAt:  time.Now().UTC(),
			FinishedAt: time.Now().UTC(),
			Metrics:    map[string]float64{},
		},
	}

	alerts = engine.Evaluate(results2)
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}

func TestAlertRuleEngine_Evaluate_Cooldown(t *testing.T) {
	rules := []AlertRule{
		{
			ID:       "rule-1",
			Name:     "Cooldown Test",
			Enabled:  true,
			CheckIDs: []string{"check-1"},
			Conditions: []AlertCondition{
				{
					Field:    "status",
					Operator: OperatorEquals,
					Value:    "critical",
				},
			},
			Severity:        "warning",
			Channels:        []AlertChannel{{Type: "log"}},
			CooldownMinutes: 5,
		},
	}

	logger := &mockLogger{}
	engine := NewAlertRuleEngine(rules, logger)

	result := CheckResult{
		ID:         "result-1",
		CheckID:    "check-1",
		Name:       "Test Check",
		Status:     "critical",
		Healthy:    false,
		DurationMs: 100,
		StartedAt:  time.Now().UTC(),
		FinishedAt: time.Now().UTC(),
		Metrics:    map[string]float64{},
	}

	// First evaluation should trigger
	alerts := engine.Evaluate([]CheckResult{result})
	if len(alerts) != 1 {
		t.Errorf("expected 1 alert on first evaluation, got %d", len(alerts))
	}

	// Immediate second evaluation should not trigger (cooldown)
	alerts = engine.Evaluate([]CheckResult{result})
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts during cooldown, got %d", len(alerts))
	}
}

func TestAlertRuleEngine_Evaluate_DisabledRule(t *testing.T) {
	rules := []AlertRule{
		{
			ID:       "rule-1",
			Name:     "Disabled Rule",
			Enabled:  false,
			CheckIDs: []string{"check-1"},
			Conditions: []AlertCondition{
				{
					Field:    "status",
					Operator: OperatorEquals,
					Value:    "critical",
				},
			},
			Severity:        "warning",
			Channels:        []AlertChannel{{Type: "log"}},
			CooldownMinutes: 0,
		},
	}

	logger := &mockLogger{}
	engine := NewAlertRuleEngine(rules, logger)

	results := []CheckResult{
		{
			ID:         "result-1",
			CheckID:    "check-1",
			Name:       "Test Check",
			Status:     "critical",
			Healthy:    false,
			DurationMs: 100,
			StartedAt:  time.Now().UTC(),
			FinishedAt: time.Now().UTC(),
			Metrics:    map[string]float64{},
		},
	}

	alerts := engine.Evaluate(results)
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts from disabled rule, got %d", len(alerts))
	}
}

func TestAlertRuleEngine_Evaluate_CheckIDFilter(t *testing.T) {
	rules := []AlertRule{
		{
			ID:       "rule-1",
			Name:     "Specific Check Alert",
			Enabled:  true,
			CheckIDs: []string{"check-1"}, // Only applies to check-1
			Conditions: []AlertCondition{
				{
					Field:    "status",
					Operator: OperatorEquals,
					Value:    "critical",
				},
			},
			Severity:        "warning",
			Channels:        []AlertChannel{{Type: "log"}},
			CooldownMinutes: 0,
		},
	}

	logger := &mockLogger{}
	engine := NewAlertRuleEngine(rules, logger)

	results := []CheckResult{
		{
			ID:         "result-1",
			CheckID:    "check-1",
			Name:       "Check 1",
			Status:     "critical",
			Healthy:    false,
			DurationMs: 100,
			StartedAt:  time.Now().UTC(),
			FinishedAt: time.Now().UTC(),
			Metrics:    map[string]float64{},
		},
		{
			ID:         "result-2",
			CheckID:    "check-2",
			Name:       "Check 2",
			Status:     "critical",
			Healthy:    false,
			DurationMs: 100,
			StartedAt:  time.Now().UTC(),
			FinishedAt: time.Now().UTC(),
			Metrics:    map[string]float64{},
		},
	}

	alerts := engine.Evaluate(results)
	if len(alerts) != 1 {
		t.Errorf("expected 1 alert (only for check-1), got %d", len(alerts))
	}

	if alerts[0].CheckID != "check-1" {
		t.Errorf("expected alert for check-1, got %s", alerts[0].CheckID)
	}
}

func TestAlertRuleEngine_Evaluate_HealthyCondition(t *testing.T) {
	rules := []AlertRule{
		{
			ID:       "rule-1",
			Name:     "Unhealthy Alert",
			Enabled:  true,
			CheckIDs: []string{},
			Conditions: []AlertCondition{
				{
					Field:    "healthy",
					Operator: OperatorEquals,
					Value:    false,
				},
			},
			Severity:        "warning",
			Channels:        []AlertChannel{{Type: "log"}},
			CooldownMinutes: 0,
		},
	}

	logger := &mockLogger{}
	engine := NewAlertRuleEngine(rules, logger)

	results := []CheckResult{
		{
			ID:         "result-1",
			CheckID:    "check-1",
			Name:       "Test Check",
			Status:     "warning",
			Healthy:    false,
			DurationMs: 100,
			StartedAt:  time.Now().UTC(),
			FinishedAt: time.Now().UTC(),
			Metrics:    map[string]float64{},
		},
	}

	alerts := engine.Evaluate(results)
	if len(alerts) != 1 {
		t.Errorf("expected 1 alert for unhealthy check, got %d", len(alerts))
	}
}

func TestAlertRuleEngine_Evaluate_DurationMsCondition(t *testing.T) {
	rules := []AlertRule{
		{
			ID:       "rule-1",
			Name:     "Slow Response Alert",
			Enabled:  true,
			CheckIDs: []string{},
			Conditions: []AlertCondition{
				{
					Field:    "durationMs",
					Operator: OperatorGreaterThan,
					Value:    1000,
				},
			},
			Severity:        "warning",
			Channels:        []AlertChannel{{Type: "log"}},
			CooldownMinutes: 0,
		},
	}

	logger := &mockLogger{}
	engine := NewAlertRuleEngine(rules, logger)

	results := []CheckResult{
		{
			ID:         "result-1",
			CheckID:    "check-1",
			Name:       "Test Check",
			Status:     "healthy",
			Healthy:    true,
			DurationMs: 1500,
			StartedAt:  time.Now().UTC(),
			FinishedAt: time.Now().UTC(),
			Metrics:    map[string]float64{},
		},
	}

	alerts := engine.Evaluate(results)
	if len(alerts) != 1 {
		t.Errorf("expected 1 alert for slow response, got %d", len(alerts))
	}
}

func TestAlertRuleEngine_Evaluate_NotEqualsOperator(t *testing.T) {
	rules := []AlertRule{
		{
			ID:       "rule-1",
			Name:     "Not Healthy Alert",
			Enabled:  true,
			CheckIDs: []string{},
			Conditions: []AlertCondition{
				{
					Field:    "status",
					Operator: OperatorNotEquals,
					Value:    "healthy",
				},
			},
			Severity:        "warning",
			Channels:        []AlertChannel{{Type: "log"}},
			CooldownMinutes: 0,
		},
	}

	logger := &mockLogger{}
	engine := NewAlertRuleEngine(rules, logger)

	// Should trigger: status is "warning", not "healthy"
	results := []CheckResult{
		{
			ID:         "result-1",
			CheckID:    "check-1",
			Name:       "Test Check",
			Status:     "warning",
			Healthy:    false,
			DurationMs: 100,
			StartedAt:  time.Now().UTC(),
			FinishedAt: time.Now().UTC(),
			Metrics:    map[string]float64{},
		},
	}

	alerts := engine.Evaluate(results)
	if len(alerts) != 1 {
		t.Errorf("expected 1 alert, got %d", len(alerts))
	}
}

func TestAlertRuleEngine_Evaluate_LessThanOperator(t *testing.T) {
	rules := []AlertRule{
		{
			ID:       "rule-1",
			Name:     "Low Metric Alert",
			Enabled:  true,
			CheckIDs: []string{},
			Conditions: []AlertCondition{
				{
					Field:    "durationMs",
					Operator: OperatorLessThan,
					Value:    50,
				},
			},
			Severity:        "info",
			Channels:        []AlertChannel{{Type: "log"}},
			CooldownMinutes: 0,
		},
	}

	logger := &mockLogger{}
	engine := NewAlertRuleEngine(rules, logger)

	results := []CheckResult{
		{
			ID:         "result-1",
			CheckID:    "check-1",
			Name:       "Test Check",
			Status:     "healthy",
			Healthy:    true,
			DurationMs: 30,
			StartedAt:  time.Now().UTC(),
			FinishedAt: time.Now().UTC(),
			Metrics:    map[string]float64{},
		},
	}

	alerts := engine.Evaluate(results)
	if len(alerts) != 1 {
		t.Errorf("expected 1 alert, got %d", len(alerts))
	}
}

func TestAlertRuleEngine_SendAlert(t *testing.T) {
	rules := []AlertRule{
		{
			ID:       "rule-1",
			Name:     "Multi-Channel Alert",
			Enabled:  true,
			CheckIDs: []string{},
			Conditions: []AlertCondition{
				{
					Field:    "status",
					Operator: OperatorEquals,
					Value:    "critical",
				},
			},
			Severity:        "critical",
			Channels:        []AlertChannel{{Type: "log"}, {Type: "log"}},
			CooldownMinutes: 0,
		},
	}

	logger := &mockLogger{}
	engine := NewAlertRuleEngine(rules, logger)

	results := []CheckResult{
		{
			ID:         "result-1",
			CheckID:    "check-1",
			Name:       "Test Check",
			Status:     "critical",
			Healthy:    false,
			DurationMs: 100,
			StartedAt:  time.Now().UTC(),
			FinishedAt: time.Now().UTC(),
			Metrics:    map[string]float64{},
		},
	}

	alerts := engine.Evaluate(results)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}

	// Should have logged to 2 channels
	if len(logger.messages) != 2 {
		t.Errorf("expected 2 log messages (2 channels), got %d", len(logger.messages))
	}

	// Check log message format
	for _, msg := range logger.messages {
		if !strings.Contains(msg, "[ALERT]") {
			t.Errorf("log message should contain [ALERT] prefix, got: %s", msg)
		}
		if !strings.Contains(msg, "fired") {
			t.Errorf("log message should contain status 'fired', got: %s", msg)
		}
	}
}

func TestAlertRuleEngine_BuildAlertMessage(t *testing.T) {
	rule := AlertRule{
		ID:          "rule-1",
		Name:        "Test Rule",
		Description: "This is a test alert",
		Severity:    "warning",
	}

	result := CheckResult{
		CheckID:    "check-1",
		Name:       "Test Check",
		Status:     "critical",
		Healthy:    false,
		Message:    "Connection failed",
		DurationMs: 1500,
	}

	logger := &mockLogger{}
	engine := NewAlertRuleEngine([]AlertRule{rule}, logger)

	alert := engine.buildAlert(rule, result, time.Now().UTC())

	message := alert.Message

	// Check message components
	if !strings.Contains(message, "Rule: Test Rule") {
		t.Errorf("message should contain rule name, got: %s", message)
	}
	if !strings.Contains(message, "Check: Test Check") {
		t.Errorf("message should contain check name, got: %s", message)
	}
	if !strings.Contains(message, "Status: critical") {
		t.Errorf("message should contain status, got: %s", message)
	}
	if !strings.Contains(message, "Details: Connection failed") {
		t.Errorf("message should contain details, got: %s", message)
	}
	if !strings.Contains(message, "Duration: 1500ms") {
		t.Errorf("message should contain duration, got: %s", message)
	}
	if !strings.Contains(message, "Description: This is a test alert") {
		t.Errorf("message should contain description, got: %s", message)
	}
}

func TestAlertCondition_Validate(t *testing.T) {
	tests := []struct {
		name        string
		cond        AlertCondition
		wantErr     bool
		errContains string
	}{
		{
			name: "valid status condition",
			cond: AlertCondition{
				Field:    "status",
				Operator: OperatorEquals,
				Value:    "critical",
			},
			wantErr: false,
		},
		{
			name: "valid healthy condition",
			cond: AlertCondition{
				Field:    "healthy",
				Operator: OperatorEquals,
				Value:    false,
			},
			wantErr: false,
		},
		{
			name: "valid durationMs condition",
			cond: AlertCondition{
				Field:    "durationMs",
				Operator: OperatorGreaterThan,
				Value:    1000,
			},
			wantErr: false,
		},
		{
			name: "missing field",
			cond: AlertCondition{
				Operator: OperatorEquals,
				Value:    "critical",
			},
			wantErr:     true,
			errContains: "field is required",
		},
		{
			name: "missing operator",
			cond: AlertCondition{
				Field: "status",
				Value: "critical",
			},
			wantErr:     true,
			errContains: "operator is required",
		},
		{
			name: "invalid operator",
			cond: AlertCondition{
				Field:    "status",
				Operator: "invalid_op",
				Value:    "critical",
			},
			wantErr:     true,
			errContains: "invalid operator",
		},
		{
			name: "invalid field",
			cond: AlertCondition{
				Field:    "invalid_field",
				Operator: OperatorEquals,
				Value:    "value",
			},
			wantErr:     true,
			errContains: "invalid field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cond.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error should contain %q, got: %v", tt.errContains, err)
				}
			}
		})
	}
}

func TestAlertRule_Validate(t *testing.T) {
	tests := []struct {
		name        string
		rule        AlertRule
		wantErr     bool
		errContains string
	}{
		{
			name: "valid rule",
			rule: AlertRule{
				ID:       "rule-1",
				Name:     "Test Rule",
				Enabled:  true,
				CheckIDs: []string{"check-1"},
				Conditions: []AlertCondition{
					{
						Field:    "status",
						Operator: OperatorEquals,
						Value:    "critical",
					},
				},
				Severity:        "critical",
				Channels:        []AlertChannel{{Type: "log"}},
				CooldownMinutes: 5,
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			rule: AlertRule{
				Name: "Test Rule",
				Conditions: []AlertCondition{
					{
						Field:    "status",
						Operator: OperatorEquals,
						Value:    "critical",
					},
				},
			},
			wantErr:     true,
			errContains: "rule ID is required",
		},
		{
			name: "missing name",
			rule: AlertRule{
				ID: "rule-1",
				Conditions: []AlertCondition{
					{
						Field:    "status",
						Operator: OperatorEquals,
						Value:    "critical",
					},
				},
			},
			wantErr:     true,
			errContains: "rule name is required",
		},
		{
			name: "no conditions",
			rule: AlertRule{
				ID:         "rule-1",
				Name:       "Test Rule",
				Conditions: []AlertCondition{},
			},
			wantErr:     true,
			errContains: "at least one condition",
		},
		{
			name: "negative cooldown",
			rule: AlertRule{
				ID:   "rule-1",
				Name: "Test Rule",
				Conditions: []AlertCondition{
					{
						Field:    "status",
						Operator: OperatorEquals,
						Value:    "critical",
					},
				},
				CooldownMinutes: -1,
			},
			wantErr:     true,
			errContains: "cooldown minutes must be >= 0",
		},
		{
			name: "sets default severity",
			rule: AlertRule{
				ID:   "rule-1",
				Name: "Test Rule",
				Conditions: []AlertCondition{
					{
						Field:    "status",
						Operator: OperatorEquals,
						Value:    "critical",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "sets default cooldown",
			rule: AlertRule{
				ID:   "rule-1",
				Name: "Test Rule",
				Conditions: []AlertCondition{
					{
						Field:    "status",
						Operator: OperatorEquals,
						Value:    "critical",
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.rule.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("error should contain %q, got: %v", tt.errContains, err)
				}
			}
		})
	}
}

func TestNewAlertRuleEngine(t *testing.T) {
	rules := []AlertRule{
		{
			ID:   "rule-1",
			Name: "Test Rule",
			Conditions: []AlertCondition{
				{
					Field:    "status",
					Operator: OperatorEquals,
					Value:    "critical",
				},
			},
			Channels: []AlertChannel{{Type: "log"}},
		},
	}

	logger := &mockLogger{}
	engine := NewAlertRuleEngine(rules, logger)

	if engine == nil {
		t.Fatal("NewAlertRuleEngine() returned nil")
	}

	if engine.rules == nil {
		t.Error("engine.rules is nil")
	}

	if len(engine.rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(engine.rules))
	}

	if engine.lastTriggered == nil {
		t.Error("engine.lastTriggered is nil")
	}
}

func TestAlertRuleEngine_IntegrationWithStandardLogger(t *testing.T) {
	// Test that the engine works with standard log.Logger
	rules := []AlertRule{
		{
			ID:       "rule-1",
			Name:     "Test Rule",
			Enabled:  true,
			CheckIDs: []string{},
			Conditions: []AlertCondition{
				{
					Field:    "status",
					Operator: OperatorEquals,
					Value:    "critical",
				},
			},
			Severity:        "warning",
			Channels:        []AlertChannel{{Type: "log"}},
			CooldownMinutes: 0,
		},
	}

	// Use actual log.Logger to verify compatibility
	logger := log.New(os.Stdout, "[TEST] ", log.LstdFlags)
	engine := NewAlertRuleEngine(rules, logger)

	results := []CheckResult{
		{
			ID:         "result-1",
			CheckID:    "check-1",
			Name:       "Test Check",
			Status:     "critical",
			Healthy:    false,
			DurationMs: 100,
			StartedAt:  time.Now().UTC(),
			FinishedAt: time.Now().UTC(),
			Metrics:    map[string]float64{},
		},
	}

	// Should not panic
	alerts := engine.Evaluate(results)
	if len(alerts) != 1 {
		t.Errorf("expected 1 alert, got %d", len(alerts))
	}
}
