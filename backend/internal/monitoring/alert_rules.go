package monitoring

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// AlertRuleEngine evaluates check results and generates alerts.
type AlertRuleEngine struct {
	rules         []AlertRule
	mu            sync.RWMutex
	lastTriggered map[string]time.Time // ruleID+checkID -> last triggered time
	logger        Logger
}

// Logger defines the logging interface used by the alert engine.
type Logger interface {
	Printf(format string, v ...interface{})
}

// NewAlertRuleEngine creates a new alert rule engine.
func NewAlertRuleEngine(rules []AlertRule, logger Logger) *AlertRuleEngine {
	return &AlertRuleEngine{
		rules:         rules,
		lastTriggered: make(map[string]time.Time),
		logger:        logger,
	}
}

// SetFilePath is retained for older tests/integration points but alert rules
// are no longer persisted to local files.
func (e *AlertRuleEngine) SetFilePath(path string) {
}

// PersistIfNeeded is a no-op. Runtime persistence must be provided by MongoDB.
func (e *AlertRuleEngine) PersistIfNeeded() {
}

// persist is intentionally a no-op so alert rule edits never create JSON files.
func (e *AlertRuleEngine) persist() {
}

// Evaluate processes check results and returns triggered alerts.
func (e *AlertRuleEngine) Evaluate(results []CheckResult) []Alert {
	e.mu.Lock()
	defer e.mu.Unlock()

	alerts := make([]Alert, 0)
	now := time.Now().UTC()

	for _, result := range results {
		// Find matching rules for this check
		for _, rule := range e.rules {
			if !rule.Enabled {
				continue
			}

			// Check if this rule applies to the check
			if !e.checkMatchesRule(result, rule) {
				continue
			}

			// Check cooldown
			cooldownKey := fmt.Sprintf("%s:%s", rule.ID, result.CheckID)
			if lastTriggered, ok := e.lastTriggered[cooldownKey]; ok {
				cooldownDuration := time.Duration(rule.CooldownMinutes) * time.Minute
				if now.Sub(lastTriggered) < cooldownDuration {
					continue // still in cooldown
				}
			}

			// Evaluate all conditions (AND logic)
			if e.evaluateConditions(result, rule.Conditions) {
				alert := e.buildAlert(rule, result, now)
				alerts = append(alerts, alert)
				e.lastTriggered[cooldownKey] = now
				e.sendAlert(alert, rule.Channels)
			}
		}
	}

	return alerts
}

// checkMatchesRule checks if a rule applies to a given check result.
func (e *AlertRuleEngine) checkMatchesRule(result CheckResult, rule AlertRule) bool {
	if len(rule.CheckIDs) == 0 {
		return true // applies to all checks
	}

	for _, checkID := range rule.CheckIDs {
		if checkID == result.CheckID {
			return true
		}
	}
	return false
}

// evaluateConditions checks if all conditions match (AND logic).
func (e *AlertRuleEngine) evaluateConditions(result CheckResult, conditions []AlertCondition) bool {
	if len(conditions) == 0 {
		return false // no conditions means never trigger
	}

	for _, cond := range conditions {
		if !e.evaluateCondition(result, cond) {
			return false // any condition fails = rule doesn't match
		}
	}
	return true // all conditions passed
}

// evaluateCondition checks if a single condition matches.
func (e *AlertRuleEngine) evaluateCondition(result CheckResult, cond AlertCondition) bool {
	switch cond.Field {
	case "status":
		return e.compareStrings(result.Status, cond.Operator, cond.Value)
	case "healthy":
		return e.compareBool(result.Healthy, cond.Operator, cond.Value)
	case "durationMs":
		return e.compareInt(result.DurationMs, cond.Operator, cond.Value)
	default:
		// Check metrics
		if val, ok := result.Metrics[cond.Field]; ok {
			return e.compareFloat(val, cond.Operator, cond.Value)
		}
		return false
	}
}

// compareStrings evaluates string comparisons.
func (e *AlertRuleEngine) compareStrings(actual string, op AlertOperator, expected interface{}) bool {
	expectedStr, ok := expected.(string)
	if !ok {
		return false
	}

	switch op {
	case OperatorEquals:
		return actual == expectedStr
	case OperatorNotEquals:
		return actual != expectedStr
	default:
		return false
	}
}

// compareBool evaluates boolean comparisons.
func (e *AlertRuleEngine) compareBool(actual bool, op AlertOperator, expected interface{}) bool {
	expectedBool, ok := expected.(bool)
	if !ok {
		return false
	}

	switch op {
	case OperatorEquals:
		return actual == expectedBool
	case OperatorNotEquals:
		return actual != expectedBool
	default:
		return false
	}
}

// compareInt evaluates integer comparisons.
func (e *AlertRuleEngine) compareInt(actual int64, op AlertOperator, expected interface{}) bool {
	var expectedVal int64

	switch v := expected.(type) {
	case float64:
		expectedVal = int64(v)
	case int:
		expectedVal = int64(v)
	case int64:
		expectedVal = v
	default:
		return false
	}

	switch op {
	case OperatorEquals:
		return actual == expectedVal
	case OperatorNotEquals:
		return actual != expectedVal
	case OperatorGreaterThan:
		return actual > expectedVal
	case OperatorLessThan:
		return actual < expectedVal
	default:
		return false
	}
}

// compareFloat evaluates float comparisons.
func (e *AlertRuleEngine) compareFloat(actual float64, op AlertOperator, expected interface{}) bool {
	var expectedVal float64

	switch v := expected.(type) {
	case float64:
		expectedVal = v
	case int:
		expectedVal = float64(v)
	case int64:
		expectedVal = float64(v)
	default:
		return false
	}

	switch op {
	case OperatorEquals:
		return actual == expectedVal
	case OperatorNotEquals:
		return actual != expectedVal
	case OperatorGreaterThan:
		return actual > expectedVal
	case OperatorLessThan:
		return actual < expectedVal
	default:
		return false
	}
}

// buildAlert creates an alert from a rule and check result.
func (e *AlertRuleEngine) buildAlert(rule AlertRule, result CheckResult, triggeredAt time.Time) Alert {
	message := e.buildAlertMessage(rule, result)

	return Alert{
		ID:          fmt.Sprintf("alert-%s-%s-%d", rule.ID, result.CheckID, triggeredAt.UnixNano()),
		RuleID:      rule.ID,
		RuleName:    rule.Name,
		CheckID:     result.CheckID,
		CheckName:   result.Name,
		Severity:    rule.Severity,
		Status:      "fired",
		Message:     message,
		TriggeredAt: triggeredAt,
	}
}

// buildAlertMessage creates a human-readable alert message.
func (e *AlertRuleEngine) buildAlertMessage(rule AlertRule, result CheckResult) string {
	var parts []string

	parts = append(parts, fmt.Sprintf("Rule: %s", rule.Name))
	parts = append(parts, fmt.Sprintf("Check: %s", result.Name))
	parts = append(parts, fmt.Sprintf("Status: %s", result.Status))

	if result.Message != "" {
		parts = append(parts, fmt.Sprintf("Details: %s", result.Message))
	}

	if result.DurationMs > 0 {
		parts = append(parts, fmt.Sprintf("Duration: %dms", result.DurationMs))
	}

	if rule.Description != "" {
		parts = append(parts, fmt.Sprintf("Description: %s", rule.Description))
	}

	return strings.Join(parts, " | ")
}

// sendAlert sends an alert to all configured channels.
func (e *AlertRuleEngine) sendAlert(alert Alert, channels []AlertChannel) {
	for _, channel := range channels {
		e.sendToChannel(alert, channel)
	}
}

// sendToChannel sends an alert to a specific channel.
func (e *AlertRuleEngine) sendToChannel(alert Alert, channel AlertChannel) {
	switch channel.Type {
	case "log":
		e.logger.Printf("[ALERT] %s: %s (Rule: %s, Check: %s, Severity: %s)",
			alert.Status, alert.Message, alert.RuleName, alert.CheckName, alert.Severity)
	// Additional channel types (webhook, email, etc.) can be added here
	default:
		e.logger.Printf("[ALERT] Unknown channel type %s for alert %s", channel.Type, alert.ID)
	}
}

// LoadRulesFromConfig returns built-in starter alert rules. MongoDB-backed rule
// persistence is wired explicitly by runtime repositories.
func LoadRulesFromConfig(cfg interface{}) ([]AlertRule, error) {
	return DefaultAlertRules(), nil
}

// DefaultAlertRules returns practical out-of-the-box alert rules.
func DefaultAlertRules() []AlertRule {
	return []AlertRule{
		{
			ID:              "check-down-critical",
			Name:            "Check Down",
			Enabled:         true,
			CheckIDs:        nil, // applies to all checks
			Severity:        "critical",
			CooldownMinutes: 5,
			Description:     "Fires when any health check reports unhealthy",
			Channels:        []AlertChannel{{Type: "log"}},
			Conditions: []AlertCondition{
				{Field: "healthy", Operator: OperatorEquals, Value: false},
			},
		},
		{
			ID:              "high-latency-warning",
			Name:            "High Latency",
			Enabled:         true,
			CheckIDs:        nil,
			Severity:        "warning",
			CooldownMinutes: 10,
			Description:     "Fires when response time exceeds 3 seconds",
			Channels:        []AlertChannel{{Type: "log"}},
			Conditions: []AlertCondition{
				{Field: "durationMs", Operator: OperatorGreaterThan, Value: float64(3000)},
			},
		},
		{
			ID:              "critical-latency",
			Name:            "Critical Latency",
			Enabled:         true,
			CheckIDs:        nil,
			Severity:        "critical",
			CooldownMinutes: 5,
			Description:     "Fires when response time exceeds 10 seconds",
			Channels:        []AlertChannel{{Type: "log"}},
			Conditions: []AlertCondition{
				{Field: "durationMs", Operator: OperatorGreaterThan, Value: float64(10000)},
			},
		},
		{
			ID:              "api-error-status",
			Name:            "API Error Response",
			Enabled:         true,
			CheckIDs:        nil,
			Severity:        "critical",
			CooldownMinutes: 5,
			Description:     "Fires when a check returns error status",
			Channels:        []AlertChannel{{Type: "log"}},
			Conditions: []AlertCondition{
				{Field: "status", Operator: OperatorEquals, Value: "error"},
			},
		},
		{
			ID:              "timeout-warning",
			Name:            "Check Timeout",
			Enabled:         true,
			CheckIDs:        nil,
			Severity:        "warning",
			CooldownMinutes: 10,
			Description:     "Fires when a check times out",
			Channels:        []AlertChannel{{Type: "log"}},
			Conditions: []AlertCondition{
				{Field: "status", Operator: OperatorEquals, Value: "timeout"},
			},
		},
	}
}

// Validate checks if an alert rule is valid.
func (r *AlertRule) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("rule ID is required")
	}
	if r.Name == "" {
		return fmt.Errorf("rule name is required")
	}
	if len(r.Conditions) == 0 {
		return fmt.Errorf("rule must have at least one condition")
	}
	if r.Severity == "" {
		r.Severity = "warning" // default severity
	}
	if r.CooldownMinutes < 0 {
		return fmt.Errorf("cooldown minutes must be >= 0")
	}
	if r.CooldownMinutes == 0 {
		r.CooldownMinutes = 5 // default 5 minute cooldown
	}

	// Validate conditions
	for i, cond := range r.Conditions {
		if err := cond.Validate(); err != nil {
			return fmt.Errorf("condition %d: %w", i, err)
		}
	}

	// Validate channels
	for i, ch := range r.Channels {
		if ch.Type == "" {
			return fmt.Errorf("channel %d: type is required", i)
		}
	}

	return nil
}

// Validate checks if an alert condition is valid.
func (c *AlertCondition) Validate() error {
	if c.Field == "" {
		return fmt.Errorf("field is required")
	}
	if c.Operator == "" {
		return fmt.Errorf("operator is required")
	}

	// Validate operator
	validOps := map[AlertOperator]bool{
		OperatorEquals:      true,
		OperatorNotEquals:   true,
		OperatorGreaterThan: true,
		OperatorLessThan:    true,
	}
	if !validOps[c.Operator] {
		return fmt.Errorf("invalid operator %q", c.Operator)
	}

	// Validate field
	validFields := map[string]bool{
		"status":     true,
		"healthy":    true,
		"durationMs": true,
	}
	if !validFields[c.Field] && !strings.HasPrefix(c.Field, "metrics.") {
		return fmt.Errorf("invalid field %q", c.Field)
	}

	return nil
}
