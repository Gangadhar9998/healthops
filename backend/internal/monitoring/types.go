package monitoring

import (
	"context"
	"time"
)

type Config struct {
	Server               ServerConfig  `json:"server"`
	Auth                 AuthConfig    `json:"auth"`
	RetentionDays        int           `json:"retentionDays"`
	CheckIntervalSeconds int           `json:"checkIntervalSeconds"`
	Workers              int           `json:"workers"`
	AllowCommandChecks   bool          `json:"allowCommandChecks"`
	Checks               []CheckConfig `json:"checks"`
}

type ServerConfig struct {
	Addr                string `json:"addr"`
	ReadTimeoutSeconds  int    `json:"readTimeoutSeconds"`
	WriteTimeoutSeconds int    `json:"writeTimeoutSeconds"`
	IdleTimeoutSeconds  int    `json:"idleTimeoutSeconds"`
}

type AuthConfig struct {
	Enabled  bool   `json:"enabled"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

type CheckConfig struct {
	ID                 string            `json:"id" bson:"id"`
	Name               string            `json:"name" bson:"name"`
	Type               string            `json:"type" bson:"type"`
	Server             string            `json:"server,omitempty" bson:"server,omitempty"`
	Application        string            `json:"application,omitempty" bson:"application,omitempty"`
	Target             string            `json:"target,omitempty" bson:"target,omitempty"`
	Host               string            `json:"host,omitempty" bson:"host,omitempty"`
	Port               int               `json:"port,omitempty" bson:"port,omitempty"`
	Command            string            `json:"command,omitempty" bson:"command,omitempty"`
	Path               string            `json:"path,omitempty" bson:"path,omitempty"`
	ExpectedStatus     int               `json:"expectedStatus,omitempty" bson:"expectedStatus,omitempty"`
	ExpectedContains   string            `json:"expectedContains,omitempty" bson:"expectedContains,omitempty"`
	TimeoutSeconds     int               `json:"timeoutSeconds,omitempty" bson:"timeoutSeconds,omitempty"`
	WarningThresholdMs int               `json:"warningThresholdMs,omitempty" bson:"warningThresholdMs,omitempty"`
	FreshnessSeconds   int               `json:"freshnessSeconds,omitempty" bson:"freshnessSeconds,omitempty"`
	IntervalSeconds    int               `json:"intervalSeconds,omitempty" bson:"intervalSeconds,omitempty"`
	RetryCount         int               `json:"retryCount,omitempty" bson:"retryCount,omitempty"`
	RetryDelaySeconds  int               `json:"retryDelaySeconds,omitempty" bson:"retryDelaySeconds,omitempty"`
	CooldownSeconds    int               `json:"cooldownSeconds,omitempty" bson:"cooldownSeconds,omitempty"`
	Enabled            *bool             `json:"enabled,omitempty" bson:"enabled,omitempty"`
	Tags               []string          `json:"tags,omitempty" bson:"tags,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty" bson:"metadata,omitempty"`
	MySQL              *MySQLCheckConfig `json:"mysql,omitempty" bson:"mysql,omitempty"`
}

type State struct {
	Checks    []CheckConfig `json:"checks" bson:"checks"`
	Results   []CheckResult `json:"results" bson:"results"`
	LastRunAt time.Time     `json:"lastRunAt,omitempty" bson:"lastRunAt,omitempty"`
	UpdatedAt time.Time     `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
}

type DashboardSnapshot struct {
	State       State     `json:"state" bson:"state"`
	Summary     Summary   `json:"summary" bson:"summary"`
	GeneratedAt time.Time `json:"generatedAt" bson:"generatedAt"`
}

type CheckResult struct {
	ID          string             `json:"id" bson:"_id"`
	CheckID     string             `json:"checkId" bson:"checkId"`
	Name        string             `json:"name" bson:"name"`
	Type        string             `json:"type" bson:"type"`
	Server      string             `json:"server,omitempty" bson:"server,omitempty"`
	Application string             `json:"application,omitempty" bson:"application,omitempty"`
	Status      string             `json:"status" bson:"status"`
	Healthy     bool               `json:"healthy" bson:"healthy"`
	Message     string             `json:"message,omitempty" bson:"message,omitempty"`
	DurationMs  int64              `json:"durationMs" bson:"durationMs"`
	StartedAt   time.Time          `json:"startedAt" bson:"startedAt"`
	FinishedAt  time.Time          `json:"finishedAt" bson:"finishedAt"`
	Metrics     map[string]float64 `json:"metrics,omitempty" bson:"metrics,omitempty"`
	Tags        []string           `json:"tags,omitempty" bson:"tags,omitempty"`
}

type StatusCount struct {
	Total    int `json:"total"`
	Healthy  int `json:"healthy"`
	Warning  int `json:"warning"`
	Critical int `json:"critical"`
	Unknown  int `json:"unknown"`
}

type Summary struct {
	TotalChecks   int                    `json:"totalChecks"`
	EnabledChecks int                    `json:"enabledChecks"`
	Healthy       int                    `json:"healthy"`
	Warning       int                    `json:"warning"`
	Critical      int                    `json:"critical"`
	Unknown       int                    `json:"unknown"`
	LastRunAt     *time.Time             `json:"lastRunAt,omitempty"`
	ByServer      map[string]StatusCount `json:"byServer"`
	ByApplication map[string]StatusCount `json:"byApplication"`
	Latest        []CheckResult          `json:"latest"`
}

type RunSummary struct {
	StartedAt  time.Time     `json:"startedAt"`
	FinishedAt time.Time     `json:"finishedAt"`
	Skipped    bool          `json:"skipped,omitempty"`
	Results    []CheckResult `json:"results"`
	Summary    Summary       `json:"summary"`
}

type Store interface {
	Snapshot() State
	DashboardSnapshot() DashboardSnapshot
	Update(func(*State) error) error
	ReplaceChecks([]CheckConfig) error
	UpsertCheck(CheckConfig) error
	DeleteCheck(string) error
	AppendResults([]CheckResult, int) error
	SetLastRun(time.Time) error
}

type Incident struct {
	ID             string            `json:"id" bson:"_id"`
	CheckID        string            `json:"checkId" bson:"checkId"`
	CheckName      string            `json:"checkName" bson:"checkName"`
	Type           string            `json:"type" bson:"type"`
	Status         string            `json:"status" bson:"status"`     // open, acknowledged, resolved
	Severity       string            `json:"severity" bson:"severity"` // warning, critical
	Message        string            `json:"message" bson:"message"`
	StartedAt      time.Time         `json:"startedAt" bson:"startedAt"`
	UpdatedAt      time.Time         `json:"updatedAt" bson:"updatedAt"`
	ResolvedAt     *time.Time        `json:"resolvedAt,omitempty" bson:"resolvedAt,omitempty"`
	AcknowledgedAt *time.Time        `json:"acknowledgedAt,omitempty" bson:"acknowledgedAt,omitempty"`
	AcknowledgedBy string            `json:"acknowledgedBy,omitempty" bson:"acknowledgedBy,omitempty"`
	ResolvedBy     string            `json:"resolvedBy,omitempty" bson:"resolvedBy,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty" bson:"metadata,omitempty"`
}

type Mirror interface {
	SyncState(context.Context, State) error
	ReadState(context.Context) (State, error)
	ReadDashboardSnapshot(context.Context) (DashboardSnapshot, error)
}

// AlertOperator defines comparison operators for alert conditions.
type AlertOperator string

const (
	OperatorEquals      AlertOperator = "equals"
	OperatorNotEquals   AlertOperator = "not_equals"
	OperatorGreaterThan AlertOperator = "greater_than"
	OperatorLessThan    AlertOperator = "less_than"
)

// AlertCondition defines a single condition to evaluate.
type AlertCondition struct {
	Field    string        `json:"field" bson:"field"`
	Operator AlertOperator `json:"operator" bson:"operator"`
	Value    interface{}   `json:"value" bson:"value"`
}

// AlertChannel defines where to send alerts.
type AlertChannel struct {
	Type   string                 `json:"type" bson:"type"`
	Config map[string]interface{} `json:"config" bson:"config"`
}

// AlertRule defines when and how to trigger alerts.
type AlertRule struct {
	ID                  string           `json:"id" bson:"id"`
	Name                string           `json:"name" bson:"name"`
	Enabled             bool             `json:"enabled" bson:"enabled"`
	CheckIDs            []string         `json:"checkIds" bson:"checkIds"`
	Conditions          []AlertCondition `json:"conditions" bson:"conditions"`
	Severity            string           `json:"severity" bson:"severity"`
	Channels            []AlertChannel   `json:"channels" bson:"channels"`
	CooldownMinutes     int              `json:"cooldownMinutes" bson:"cooldownMinutes"`
	Description         string           `json:"description,omitempty" bson:"description,omitempty"`
	ConsecutiveBreaches int              `json:"consecutiveBreaches,omitempty" bson:"consecutiveBreaches,omitempty"`
	RecoverySamples     int              `json:"recoverySamples,omitempty" bson:"recoverySamples,omitempty"`
	ThresholdNum        float64          `json:"thresholdNum,omitempty" bson:"thresholdNum,omitempty"`
	RuleCode            string           `json:"ruleCode,omitempty" bson:"ruleCode,omitempty"`
}

// Alert represents a triggered alert.
type Alert struct {
	ID          string     `json:"id" bson:"_id"`
	RuleID      string     `json:"ruleId" bson:"ruleId"`
	RuleName    string     `json:"ruleName" bson:"ruleName"`
	CheckID     string     `json:"checkId" bson:"checkId"`
	CheckName   string     `json:"checkName" bson:"checkName"`
	Severity    string     `json:"severity" bson:"severity"`
	Status      string     `json:"status" bson:"status"`
	Message     string     `json:"message" bson:"message"`
	TriggeredAt time.Time  `json:"triggeredAt" bson:"triggeredAt"`
	ResolvedAt  *time.Time `json:"resolvedAt,omitempty" bson:"resolvedAt,omitempty"`
}

// AlertState tracks the streak state for a rule+check pair.
type AlertState struct {
	RuleCode       string `json:"ruleCode" bson:"ruleCode"`
	CheckID        string `json:"checkId" bson:"checkId"`
	Status         string `json:"status" bson:"status"` // OK or OPEN
	BreachStreak   int    `json:"breachStreak" bson:"breachStreak"`
	RecoveryStreak int    `json:"recoveryStreak" bson:"recoveryStreak"`
	OpenIncidentID string `json:"openIncidentId,omitempty" bson:"openIncidentId,omitempty"`
}

// MySQLCheckConfig holds MySQL-specific check configuration.
type MySQLCheckConfig struct {
	DSNEnv                string `json:"dsnEnv" bson:"dsnEnv"`
	ConnectTimeoutSeconds int    `json:"connectTimeoutSeconds,omitempty" bson:"connectTimeoutSeconds,omitempty"`
	QueryTimeoutSeconds   int    `json:"queryTimeoutSeconds,omitempty" bson:"queryTimeoutSeconds,omitempty"`
	ProcesslistLimit      int    `json:"processlistLimit,omitempty" bson:"processlistLimit,omitempty"`
	StatementLimit        int    `json:"statementLimit,omitempty" bson:"statementLimit,omitempty"`
	HostUserLimit         int    `json:"hostUserLimit,omitempty" bson:"hostUserLimit,omitempty"`
}

// IncidentSnapshot holds evidence captured at incident open time.
type IncidentSnapshot struct {
	IncidentID   string    `json:"incidentId" bson:"incidentId"`
	SnapshotType string    `json:"snapshotType" bson:"snapshotType"`
	Timestamp    time.Time `json:"timestamp" bson:"timestamp"`
	PayloadJSON  string    `json:"payloadJson" bson:"payloadJson"`
}

// NotificationEvent represents a queued notification for external delivery.
type NotificationEvent struct {
	NotificationID string     `json:"notificationId" bson:"notificationId"`
	IncidentID     string     `json:"incidentId" bson:"incidentId"`
	Channel        string     `json:"channel" bson:"channel"`
	PayloadJSON    string     `json:"payloadJson" bson:"payloadJson"`
	Status         string     `json:"status" bson:"status"` // pending, sent, failed
	RetryCount     int        `json:"retryCount" bson:"retryCount"`
	LastError      string     `json:"lastError,omitempty" bson:"lastError,omitempty"`
	CreatedAt      time.Time  `json:"createdAt" bson:"createdAt"`
	SentAt         *time.Time `json:"sentAt,omitempty" bson:"sentAt,omitempty"`
}

// AIQueueItem represents a queued AI analysis request.
type AIQueueItem struct {
	IncidentID    string     `json:"incidentId" bson:"incidentId"`
	PromptVersion string     `json:"promptVersion" bson:"promptVersion"`
	Status        string     `json:"status" bson:"status"` // pending, processing, completed, failed
	CreatedAt     time.Time  `json:"createdAt" bson:"createdAt"`
	ClaimedAt     *time.Time `json:"claimedAt,omitempty" bson:"claimedAt,omitempty"`
	CompletedAt   *time.Time `json:"completedAt,omitempty" bson:"completedAt,omitempty"`
	LastError     string     `json:"lastError,omitempty" bson:"lastError,omitempty"`
}

// AIAnalysisResult holds the result of an AI analysis.
type AIAnalysisResult struct {
	IncidentID  string    `json:"incidentId" bson:"incidentId"`
	Analysis    string    `json:"analysis" bson:"analysis"`
	Suggestions []string  `json:"suggestions,omitempty" bson:"suggestions,omitempty"`
	Severity    string    `json:"severity,omitempty" bson:"severity,omitempty"`
	CreatedAt   time.Time `json:"createdAt" bson:"createdAt"`
}
