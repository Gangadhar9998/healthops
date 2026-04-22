package repositories

import (
	"context"
	"time"
)

// Repository interfaces provide database-agnostic abstractions for data access.
// Each interface defines the contract for a specific domain entity, allowing
// implementations to be swapped (MongoDB, PostgreSQL, MySQL, etc.) without
// affecting business logic.
//
// All methods accept context.Context for cancellation/timeout control and
// return standard Go error types. Repository layer is responsible for:
// - Data persistence and retrieval
// - Query execution and optimization
// - Transaction handling
// - Data mapping between domain and storage models
//
// Business logic, validation, and use-case orchestration belong in the service layer.

// Server - Server configuration entity
type Server struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	Type        string    `json:"type"` // "api", "tcp", "mysql", etc.
	Tags        []string  `json:"tags"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ServerRepository - manages server configuration data
type ServerRepository interface {
	// List retrieves all servers
	List(ctx context.Context) ([]Server, error)

	// Get retrieves a server by ID
	Get(ctx context.Context, id string) (*Server, error)

	// Create inserts a new server
	Create(ctx context.Context, server *Server) error

	// Update modifies an existing server
	Update(ctx context.Context, id string, server *Server) error

	// Delete removes a server
	Delete(ctx context.Context, id string) error

	// SeedIfEmpty populates default servers if none exist
	SeedIfEmpty(ctx context.Context, servers []Server) error
}

// NotificationChannel - Alert delivery configuration
type NotificationChannel struct {
	ID                     string            `json:"id"`
	Name                   string            `json:"name"`
	Type                   string            `json:"type"` // "email", "webhook", "slack", "telegram"
	Enabled                bool              `json:"enabled"`
	WebhookURL             string            `json:"webhookUrl,omitempty"`
	Email                  string            `json:"email,omitempty"`
	SMTPHost               string            `json:"smtpHost,omitempty"`
	SMTPPort               int               `json:"smtpPort,omitempty"`
	SMTPUser               string            `json:"smtpUser,omitempty"`
	SMTPPass               string            `json:"smtpPass,omitempty"`
	FromEmail              string            `json:"fromEmail,omitempty"`
	BotToken               string            `json:"botToken,omitempty"`
	ChatID                 string            `json:"chatId,omitempty"`
	RoutingKey             string            `json:"routingKey,omitempty"`
	Severities             []string          `json:"severities,omitempty"`
	CheckIDs               []string          `json:"checkIds,omitempty"`
	CheckTypes             []string          `json:"checkTypes,omitempty"`
	Servers                []string          `json:"servers,omitempty"`
	Tags                   []string          `json:"tags,omitempty"`
	CooldownMinutes        int               `json:"cooldownMinutes,omitempty"`
	MinConsecutiveFailures int               `json:"minConsecutiveFailures,omitempty"`
	NotifyOnResolve        bool              `json:"notifyOnResolve,omitempty"`
	Headers                map[string]string `json:"headers,omitempty"`
	CreatedAt              time.Time         `json:"created_at"`
	UpdatedAt              time.Time         `json:"updated_at"`
}

// NotificationChannelRepository - manages alert notification channels
type NotificationChannelRepository interface {
	// List retrieves all notification channels
	List(ctx context.Context) ([]NotificationChannel, error)

	// Get retrieves a channel by ID
	Get(ctx context.Context, id string) (*NotificationChannel, error)

	// Create inserts a new notification channel
	Create(ctx context.Context, channel *NotificationChannel) error

	// Update modifies an existing channel
	Update(ctx context.Context, id string, channel *NotificationChannel) error

	// Delete removes a channel
	Delete(ctx context.Context, id string) error

	// GetEnabled retrieves only active channels for alert dispatch
	GetEnabled(ctx context.Context) ([]NotificationChannel, error)
}

// User - System user with authentication credentials
type User struct {
	Username           string     `json:"username"`
	UsernameKey        string     `json:"-" bson:"usernameKey"`
	Password           string     `json:"-" bson:"password"` // hashed password, never exposed in JSON
	Role               string     `json:"role"`              // "admin", "ops"
	DisplayName        string     `json:"displayName,omitempty" bson:"displayName,omitempty"`
	Email              string     `json:"email"`
	Enabled            bool       `json:"enabled"`
	MustChangePassword bool       `json:"mustChangePassword" bson:"mustChangePassword"`
	CreatedAt          time.Time  `json:"created_at" bson:"createdAt"`
	UpdatedAt          time.Time  `json:"updated_at" bson:"updatedAt"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty" bson:"lastLoginAt,omitempty"`
}

// UserRepository - manages user accounts and authentication
type UserRepository interface {
	// FindByUsername retrieves a user by username (for auth)
	FindByUsername(ctx context.Context, username string) (*User, error)

	// List retrieves all users
	List(ctx context.Context) ([]User, error)

	// Create inserts a new user
	Create(ctx context.Context, user *User) error

	// Update modifies an existing user
	Update(ctx context.Context, username string, user *User) error

	// Delete removes a user
	Delete(ctx context.Context, username string) error

	// BootstrapAdmin creates or resets the admin user when explicitly requested.
	BootstrapAdmin(ctx context.Context, password, email string, forceReset bool) (bool, error)
}

// AlertRule - Configurable alert threshold and condition
type AlertRule struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Metric    string                 `json:"metric"`    // "cpu_usage", "response_time", etc.
	Condition string                 `json:"condition"` // "gt", "lt", "eq"
	Threshold float64                `json:"threshold"` // value to compare against
	Duration  time.Duration          `json:"duration"`  // how long condition must be true
	Enabled   bool                   `json:"enabled"`
	Severity  string                 `json:"severity"` // "critical", "warning", "info"
	Actions   []string               `json:"actions"`  // ["email", "webhook"]
	Metadata  map[string]interface{} `json:"metadata"` // custom rule properties
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// AlertRuleRepository - manages alert rule configurations
type AlertRuleRepository interface {
	// List retrieves all alert rules
	List(ctx context.Context) ([]AlertRule, error)

	// Get retrieves a rule by ID
	Get(ctx context.Context, id string) (*AlertRule, error)

	// Create inserts a new alert rule
	Create(ctx context.Context, rule *AlertRule) error

	// Update modifies an existing rule
	Update(ctx context.Context, id string, rule *AlertRule) error

	// Delete removes a rule
	Delete(ctx context.Context, id string) error

	// GetEnabled retrieves only active rules for evaluation
	GetEnabled(ctx context.Context) ([]AlertRule, error)
}

// AIProvider - AI provider configuration for BYOK AI analysis
type AIProvider struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Provider     string            `json:"provider"`   // "openai", "anthropic", "gemini", "ollama", "custom"
	APIKey       string            `json:"api_key"`    // encrypted at rest
	BaseURL      string            `json:"base_url"`   // custom endpoint for custom providers
	Model        string            `json:"model"`      // model name
	IsDefault    bool              `json:"is_default"` // mark as default provider
	Enabled      bool              `json:"enabled"`
	PromptConfig map[string]string `json:"prompt_config"` // template overrides
	CreatedAt    time.Time         `json:"created_at"`
	UpdatedAt    time.Time         `json:"updated_at"`
	LastUsedAt   *time.Time        `json:"last_used_at,omitempty"`
	HealthStatus string            `json:"health_status"` // "healthy", "unhealthy", "unknown"
}

// AIConfigRepository - manages AI provider configurations
type AIConfigRepository interface {
	// List retrieves all AI providers
	List(ctx context.Context) ([]AIProvider, error)

	// Get retrieves a provider by ID
	Get(ctx context.Context, id string) (*AIProvider, error)

	// Create inserts a new AI provider
	Create(ctx context.Context, provider *AIProvider) error

	// Update modifies an existing provider
	Update(ctx context.Context, id string, provider *AIProvider) error

	// Delete removes a provider
	Delete(ctx context.Context, id string) error

	// GetDefault retrieves the default AI provider
	GetDefault(ctx context.Context) (*AIProvider, error)
}
