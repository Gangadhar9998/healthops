package notify

import (
	"fmt"
	"net"
	"net/url"
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

// ChannelStore defines the persistence operations needed by notification APIs and dispatch.
type ChannelStore interface {
	List() []NotificationChannelConfig
	ListRaw() []NotificationChannelConfig
	Get(id string) (NotificationChannelConfig, bool)
	Create(ch NotificationChannelConfig) error
	Update(id string, ch NotificationChannelConfig) error
	Delete(id string) error
	ToggleEnabled(id string, enabled bool) error
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
		if err := validateWebhookURL(c.WebhookURL); err != nil {
			return err
		}
	case ChannelWebhook:
		if c.WebhookURL == "" {
			return fmt.Errorf("webhookUrl is required for webhook")
		}
		if err := validateWebhookURL(c.WebhookURL); err != nil {
			return err
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

// validateWebhookURL validates that a webhook URL is safe (no SSRF to internal networks).
func validateWebhookURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}

	// Only allow HTTPS and HTTP schemes
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return fmt.Errorf("webhook URL must use http or https scheme")
	}

	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("webhook URL must have a hostname")
	}

	// Block localhost and loopback
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "0.0.0.0" {
		return fmt.Errorf("webhook URL must not target localhost")
	}

	// Block private/internal IP ranges
	ip := net.ParseIP(host)
	if ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("webhook URL must not target private or internal IP addresses")
		}
	}

	// Block common metadata endpoints
	if host == "169.254.169.254" || host == "metadata.google.internal" {
		return fmt.Errorf("webhook URL must not target cloud metadata services")
	}

	return nil
}
