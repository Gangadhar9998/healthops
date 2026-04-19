package monitoring

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}
	if c.Server.ReadTimeoutSeconds <= 0 {
		c.Server.ReadTimeoutSeconds = 10
	}
	if c.Server.WriteTimeoutSeconds <= 0 {
		c.Server.WriteTimeoutSeconds = 10
	}
	if c.Server.IdleTimeoutSeconds <= 0 {
		c.Server.IdleTimeoutSeconds = 60
	}
	if c.RetentionDays <= 0 {
		c.RetentionDays = 7
	}
	if c.CheckIntervalSeconds <= 0 {
		c.CheckIntervalSeconds = 60
	}
	if c.Workers <= 0 {
		c.Workers = 8
	}
	for i := range c.Checks {
		c.Checks[i].applyDefaults()
	}
}

func (c *Config) validate() error {
	if len(c.Checks) == 0 {
		return fmt.Errorf("config must define at least one check")
	}

	seen := map[string]struct{}{}
	for i := range c.Checks {
		check := &c.Checks[i]
		if check.ID == "" {
			check.ID = buildCheckID(check)
		}
		if check.ID == "" {
			return fmt.Errorf("check at index %d must have an id or name", i)
		}
		if _, exists := seen[check.ID]; exists {
			return fmt.Errorf("duplicate check id %q", check.ID)
		}
		seen[check.ID] = struct{}{}

		if err := check.validate(c); err != nil {
			return fmt.Errorf("check %q: %w", check.ID, err)
		}
	}
	return nil
}

func (c *CheckConfig) applyDefaults() {
	if c.TimeoutSeconds <= 0 {
		c.TimeoutSeconds = 5
	}
	if c.ExpectedStatus == 0 {
		c.ExpectedStatus = 200
	}
	if c.Enabled == nil {
		defaultEnabled := true
		c.Enabled = &defaultEnabled
	}
	c.Type = strings.ToLower(strings.TrimSpace(c.Type))
	if c.Server == "" {
		c.Server = "default"
	}
}

func (c *CheckConfig) validate(cfg *Config) error {
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}

	// Validate per-check scheduling configuration
	if c.IntervalSeconds > 0 && c.IntervalSeconds < 10 {
		return fmt.Errorf("intervalSeconds must be >= 10 (got %d)", c.IntervalSeconds)
	}
	if c.RetryCount < 0 {
		return fmt.Errorf("retryCount must be >= 0 (got %d)", c.RetryCount)
	}
	if c.RetryCount > 0 && c.RetryDelaySeconds < 1 {
		return fmt.Errorf("retryDelaySeconds must be >= 1 when retryCount > 0 (got %d)", c.RetryDelaySeconds)
	}
	if c.CooldownSeconds < 0 {
		return fmt.Errorf("cooldownSeconds must be >= 0 (got %d)", c.CooldownSeconds)
	}

	switch c.Type {
	case "api":
		if c.Target == "" {
			return fmt.Errorf("target is required for api checks")
		}
	case "tcp":
		if c.Port <= 0 {
			return fmt.Errorf("port is required for tcp checks")
		}
		if c.Host == "" && c.Target == "" {
			return fmt.Errorf("host or target is required for tcp checks")
		}
	case "process":
		if c.Target == "" {
			return fmt.Errorf("target is required for process checks")
		}
	case "command":
		if c.Command == "" {
			return fmt.Errorf("command is required for command checks")
		}
		if !cfg.AllowCommandChecks {
			// SECURITY: Command checks are disabled by default to prevent command injection.
			// They must be explicitly enabled via "allowCommandChecks": true in config.
			// Command checks execute arbitrary shell commands and should only be used
			// in trusted environments with carefully controlled config files.
			return fmt.Errorf("command checks are disabled for security; set allowCommandChecks=true to enable (use with caution)")
		}
	case "log":
		if c.Path == "" {
			return fmt.Errorf("path is required for log checks")
		}
		if c.FreshnessSeconds <= 0 {
			return fmt.Errorf("freshnessSeconds is required for log checks")
		}
	case "mysql":
		if c.MySQL == nil {
			return fmt.Errorf("mysql config block is required for mysql checks")
		}
		if c.MySQL.DSNEnv == "" {
			return fmt.Errorf("mysql.dsnEnv is required for mysql checks")
		}
		if c.MySQL.ConnectTimeoutSeconds <= 0 {
			c.MySQL.ConnectTimeoutSeconds = 3
		}
		if c.MySQL.QueryTimeoutSeconds <= 0 {
			c.MySQL.QueryTimeoutSeconds = 5
		}
		if c.MySQL.ProcesslistLimit <= 0 {
			c.MySQL.ProcesslistLimit = 50
		}
		if c.MySQL.StatementLimit <= 0 {
			c.MySQL.StatementLimit = 20
		}
		if c.MySQL.HostUserLimit <= 0 {
			c.MySQL.HostUserLimit = 20
		}
		if c.IntervalSeconds <= 0 {
			c.IntervalSeconds = 15
		}
		if c.TimeoutSeconds <= 0 {
			c.TimeoutSeconds = 10
		}
	default:
		return fmt.Errorf("unsupported type %q", c.Type)
	}
	return nil
}

func (c CheckConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

func buildCheckID(check *CheckConfig) string {
	base := strings.TrimSpace(check.ID)
	if base == "" {
		base = check.Name
	}
	if base == "" {
		base = check.Target
	}
	return slugify(base)
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = nonSlug.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "check"
	}
	return value
}

func resolveTCPAddress(check CheckConfig) string {
	if check.Target != "" && strings.Contains(check.Target, ":") {
		return check.Target
	}
	host := check.Host
	if host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("%s:%d", host, check.Port)
}

func ensureConfigPath(path string) string {
	if path == "" {
		return filepath.Join("config", "default.json")
	}
	return path
}
