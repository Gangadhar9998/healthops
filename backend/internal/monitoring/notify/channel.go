package notify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ChannelType identifies supported notification channel types.
type ChannelType string

const (
	ChannelSlack     ChannelType = "slack"
	ChannelWebhook   ChannelType = "webhook"
	ChannelEmail     ChannelType = "email"
	ChannelDiscord   ChannelType = "discord"
	ChannelTelegram  ChannelType = "telegram"
	ChannelPagerDuty ChannelType = "pagerduty"
)

// NotificationChannelConfig defines a configured notification channel with smart filters.
type NotificationChannelConfig struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	Type    ChannelType `json:"type"`
	Enabled bool        `json:"enabled"`

	// Connection settings (type-specific)
	WebhookURL string `json:"webhookUrl,omitempty"` // slack, webhook, discord
	Email      string `json:"email,omitempty"`      // email (comma-separated for multiple)
	SMTPHost   string `json:"smtpHost,omitempty"`
	SMTPPort   int    `json:"smtpPort,omitempty"`
	SMTPUser   string `json:"smtpUser,omitempty"`
	SMTPPass   string `json:"smtpPass,omitempty"`
	FromEmail  string `json:"fromEmail,omitempty"`
	BotToken   string `json:"botToken,omitempty"`   // telegram
	ChatID     string `json:"chatId,omitempty"`     // telegram
	RoutingKey string `json:"routingKey,omitempty"` // pagerduty

	// Smart filters — all optional, empty = match all
	Severities             []string `json:"severities,omitempty"`             // ["critical"], ["warning","critical"]
	CheckIDs               []string `json:"checkIds,omitempty"`               // only these checks
	CheckTypes             []string `json:"checkTypes,omitempty"`             // ["mysql","api"]
	Servers                []string `json:"servers,omitempty"`                // ["production","staging"]
	Tags                   []string `json:"tags,omitempty"`                   // check must have at least one matching tag
	CooldownMinutes        int      `json:"cooldownMinutes,omitempty"`        // min time between notifications for same check
	MinConsecutiveFailures int      `json:"minConsecutiveFailures,omitempty"` // require N failures before notifying
	NotifyOnResolve        bool     `json:"notifyOnResolve,omitempty"`        // also notify when incident resolves

	// Headers for custom webhooks
	Headers map[string]string `json:"headers,omitempty"`

	// Metadata
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Validate checks the channel config is valid.
func (c *NotificationChannelConfig) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	switch c.Type {
	case ChannelSlack, ChannelDiscord:
		if c.WebhookURL == "" {
			return fmt.Errorf("webhookUrl is required for %s", c.Type)
		}
	case ChannelWebhook:
		if c.WebhookURL == "" {
			return fmt.Errorf("webhookUrl is required for webhook")
		}
	case ChannelEmail:
		if c.Email == "" {
			return fmt.Errorf("email is required")
		}
		if c.SMTPHost == "" {
			return fmt.Errorf("smtpHost is required for email")
		}
	case ChannelTelegram:
		if c.BotToken == "" || c.ChatID == "" {
			return fmt.Errorf("botToken and chatId are required for telegram")
		}
	case ChannelPagerDuty:
		if c.RoutingKey == "" {
			return fmt.Errorf("routingKey is required for pagerduty")
		}
	default:
		return fmt.Errorf("unsupported channel type: %s", c.Type)
	}
	return nil
}

// SafeView returns a copy with sensitive fields masked.
func (c *NotificationChannelConfig) SafeView() NotificationChannelConfig {
	safe := *c
	if safe.SMTPPass != "" {
		safe.SMTPPass = "••••••••"
	}
	if safe.BotToken != "" {
		safe.BotToken = maskString(safe.BotToken)
	}
	if safe.RoutingKey != "" {
		safe.RoutingKey = maskString(safe.RoutingKey)
	}
	return safe
}

func maskString(s string) string {
	if len(s) <= 8 {
		return "••••••••"
	}
	return s[:4] + "••••" + s[len(s)-4:]
}

// NotificationChannelStore persists notification channel configurations.
type NotificationChannelStore struct {
	mu       sync.RWMutex
	channels []NotificationChannelConfig
	path     string
}

// NewNotificationChannelStore creates a file-backed channel store.
func NewNotificationChannelStore(dataDir string) (*NotificationChannelStore, error) {
	path := filepath.Join(dataDir, "notification_channels.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create dir: %w", err)
	}

	store := &NotificationChannelStore{path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, fmt.Errorf("read channels: %w", err)
	}

	if len(data) > 0 {
		if err := json.Unmarshal(data, &store.channels); err != nil {
			return nil, fmt.Errorf("parse channels: %w", err)
		}
	}

	return store, nil
}

// List returns all channels (safe view).
func (s *NotificationChannelStore) List() []NotificationChannelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]NotificationChannelConfig, len(s.channels))
	for i, ch := range s.channels {
		result[i] = ch.SafeView()
	}
	return result
}

// ListRaw returns all channels with sensitive data (for internal dispatch use).
func (s *NotificationChannelStore) ListRaw() []NotificationChannelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]NotificationChannelConfig, len(s.channels))
	copy(result, s.channels)
	return result
}

// Get returns a single channel by ID (safe view).
func (s *NotificationChannelStore) Get(id string) (NotificationChannelConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, ch := range s.channels {
		if ch.ID == id {
			safe := ch.SafeView()
			return safe, true
		}
	}
	return NotificationChannelConfig{}, false
}

// Create adds a new channel.
func (s *NotificationChannelStore) Create(ch NotificationChannelConfig) error {
	if err := ch.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.channels {
		if existing.ID == ch.ID {
			return fmt.Errorf("channel with id %q already exists", ch.ID)
		}
	}

	if ch.ID == "" {
		ch.ID = fmt.Sprintf("ch-%d", time.Now().UnixNano())
	}
	now := time.Now().UTC()
	ch.CreatedAt = now
	ch.UpdatedAt = now

	s.channels = append(s.channels, ch)
	return s.save()
}

// Update replaces a channel by ID.
func (s *NotificationChannelStore) Update(id string, ch NotificationChannelConfig) error {
	if err := ch.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for i, existing := range s.channels {
		if existing.ID == id {
			ch.ID = id
			ch.CreatedAt = existing.CreatedAt
			ch.UpdatedAt = time.Now().UTC()
			// Preserve sensitive fields if masked values sent back
			if ch.SMTPPass == "••••••••" {
				ch.SMTPPass = existing.SMTPPass
			}
			if ch.BotToken != "" && len(ch.BotToken) > 4 && ch.BotToken[4:8] == "••••" {
				ch.BotToken = existing.BotToken
			}
			if ch.RoutingKey != "" && len(ch.RoutingKey) > 4 && ch.RoutingKey[4:8] == "••••" {
				ch.RoutingKey = existing.RoutingKey
			}
			s.channels[i] = ch
			return s.save()
		}
	}
	return fmt.Errorf("channel not found: %s", id)
}

// Delete removes a channel by ID.
func (s *NotificationChannelStore) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, ch := range s.channels {
		if ch.ID == id {
			s.channels = append(s.channels[:i], s.channels[i+1:]...)
			return s.save()
		}
	}
	return fmt.Errorf("channel not found: %s", id)
}

// ToggleEnabled enables/disables a channel.
func (s *NotificationChannelStore) ToggleEnabled(id string, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, ch := range s.channels {
		if ch.ID == id {
			s.channels[i].Enabled = enabled
			s.channels[i].UpdatedAt = time.Now().UTC()
			return s.save()
		}
	}
	return fmt.Errorf("channel not found: %s", id)
}

func (s *NotificationChannelStore) save() error {
	data, err := json.MarshalIndent(s.channels, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal channels: %w", err)
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write channels: %w", err)
	}
	return os.Rename(tmpPath, s.path)
}
