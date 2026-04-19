package notify

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"medics-health-check/backend/internal/monitoring"
)

// ---------------------------------------------------------------------------
// 1. NotificationChannelConfig Validation
// ---------------------------------------------------------------------------

func TestNotificationChannelConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     NotificationChannelConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid slack config",
			cfg: NotificationChannelConfig{
				Name:       "slack-ops",
				Type:       ChannelSlack,
				WebhookURL: "https://hooks.slack.com/services/T00/B00/xxx",
			},
		},
		{
			name: "valid webhook config",
			cfg: NotificationChannelConfig{
				Name:       "custom-hook",
				Type:       ChannelWebhook,
				WebhookURL: "https://example.com/hook",
			},
		},
		{
			name: "valid email config",
			cfg: NotificationChannelConfig{
				Name:     "email-alerts",
				Type:     ChannelEmail,
				Email:    "ops@example.com",
				SMTPHost: "smtp.example.com",
			},
		},
		{
			name: "valid telegram config",
			cfg: NotificationChannelConfig{
				Name:     "tg-alerts",
				Type:     ChannelTelegram,
				BotToken: "123456:ABC-DEF",
				ChatID:   "-100123456",
			},
		},
		{
			name: "valid pagerduty config",
			cfg: NotificationChannelConfig{
				Name:       "pd-critical",
				Type:       ChannelPagerDuty,
				RoutingKey: "abc123routingkey",
			},
		},
		{
			name:    "missing name",
			cfg:     NotificationChannelConfig{Type: ChannelSlack, WebhookURL: "https://x"},
			wantErr: true,
			errMsg:  "name is required",
		},
		{
			name:    "slack without webhookUrl",
			cfg:     NotificationChannelConfig{Name: "s", Type: ChannelSlack},
			wantErr: true,
			errMsg:  "webhookUrl is required",
		},
		{
			name:    "email without smtpHost",
			cfg:     NotificationChannelConfig{Name: "e", Type: ChannelEmail, Email: "a@b.com"},
			wantErr: true,
			errMsg:  "smtpHost is required",
		},
		{
			name:    "telegram without botToken",
			cfg:     NotificationChannelConfig{Name: "t", Type: ChannelTelegram, ChatID: "123"},
			wantErr: true,
			errMsg:  "botToken and chatId are required",
		},
		{
			name:    "pagerduty without routingKey",
			cfg:     NotificationChannelConfig{Name: "p", Type: ChannelPagerDuty},
			wantErr: true,
			errMsg:  "routingKey is required",
		},
		{
			name:    "unknown channel type",
			cfg:     NotificationChannelConfig{Name: "x", Type: "carrier_pigeon"},
			wantErr: true,
			errMsg:  "unsupported channel type",
		},
		{
			name:    "discord without webhookUrl",
			cfg:     NotificationChannelConfig{Name: "d", Type: ChannelDiscord},
			wantErr: true,
			errMsg:  "webhookUrl is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 2. NotificationChannelConfig SafeView
// ---------------------------------------------------------------------------

func TestNotificationChannelConfig_SafeView(t *testing.T) {
	t.Run("SMTPPass gets masked", func(t *testing.T) {
		cfg := NotificationChannelConfig{SMTPPass: "supersecret123"}
		safe := cfg.SafeView()
		if safe.SMTPPass != "••••••••" {
			t.Errorf("SMTPPass = %q, want ••••••••", safe.SMTPPass)
		}
	})

	t.Run("short botToken gets fully masked", func(t *testing.T) {
		cfg := NotificationChannelConfig{BotToken: "short"}
		safe := cfg.SafeView()
		if safe.BotToken != "••••••••" {
			t.Errorf("BotToken = %q, want ••••••••", safe.BotToken)
		}
	})

	t.Run("long botToken gets partially masked", func(t *testing.T) {
		cfg := NotificationChannelConfig{BotToken: "123456789:ABCDEF_TOKEN"}
		safe := cfg.SafeView()
		want := "1234••••OKEN"
		if safe.BotToken != want {
			t.Errorf("BotToken = %q, want %q", safe.BotToken, want)
		}
	})

	t.Run("routingKey gets masked", func(t *testing.T) {
		cfg := NotificationChannelConfig{RoutingKey: "abcd"}
		safe := cfg.SafeView()
		if safe.RoutingKey != "••••••••" {
			t.Errorf("RoutingKey = %q, want ••••••••", safe.RoutingKey)
		}
	})

	t.Run("non-sensitive fields preserved", func(t *testing.T) {
		cfg := NotificationChannelConfig{
			Name:       "my-channel",
			WebhookURL: "https://example.com/hook",
			Email:      "ops@example.com",
		}
		safe := cfg.SafeView()
		if safe.Name != "my-channel" {
			t.Errorf("Name = %q, want %q", safe.Name, "my-channel")
		}
		if safe.WebhookURL != "https://example.com/hook" {
			t.Errorf("WebhookURL = %q, want preserved", safe.WebhookURL)
		}
		if safe.Email != "ops@example.com" {
			t.Errorf("Email = %q, want preserved", safe.Email)
		}
	})
}

// ---------------------------------------------------------------------------
// 3. NotificationChannelStore CRUD
// ---------------------------------------------------------------------------

func TestNotificationChannelStore(t *testing.T) {
	t.Run("initially empty", func(t *testing.T) {
		store, err := NewNotificationChannelStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if got := store.List(); len(got) != 0 {
			t.Errorf("expected empty list, got %d items", len(got))
		}
	})

	t.Run("create and list", func(t *testing.T) {
		store, err := NewNotificationChannelStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}

		ch := NotificationChannelConfig{
			ID:         "ch-1",
			Name:       "slack-ops",
			Type:       ChannelSlack,
			Enabled:    true,
			WebhookURL: "https://hooks.slack.com/x",
		}
		if err := store.Create(ch); err != nil {
			t.Fatal(err)
		}

		list := store.List()
		if len(list) != 1 {
			t.Fatalf("expected 1 channel, got %d", len(list))
		}
		if list[0].Name != "slack-ops" {
			t.Errorf("Name = %q, want %q", list[0].Name, "slack-ops")
		}
	})

	t.Run("create duplicate ID errors", func(t *testing.T) {
		store, err := NewNotificationChannelStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		ch := NotificationChannelConfig{
			ID:         "dup-1",
			Name:       "first",
			Type:       ChannelWebhook,
			WebhookURL: "https://a.com",
		}
		if err := store.Create(ch); err != nil {
			t.Fatal(err)
		}
		ch.Name = "second"
		if err := store.Create(ch); err == nil {
			t.Fatal("expected error for duplicate ID")
		}
	})

	t.Run("create with empty ID auto-generates", func(t *testing.T) {
		store, err := NewNotificationChannelStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		ch := NotificationChannelConfig{
			Name:       "auto-id",
			Type:       ChannelWebhook,
			WebhookURL: "https://b.com",
		}
		if err := store.Create(ch); err != nil {
			t.Fatal(err)
		}
		list := store.List()
		if len(list) != 1 {
			t.Fatalf("expected 1 channel, got %d", len(list))
		}
		if list[0].ID == "" {
			t.Error("expected auto-generated ID, got empty string")
		}
		if !strings.HasPrefix(list[0].ID, "ch-") {
			t.Errorf("expected ID prefix 'ch-', got %q", list[0].ID)
		}
	})

	t.Run("get by ID", func(t *testing.T) {
		store, err := NewNotificationChannelStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		ch := NotificationChannelConfig{
			ID:         "get-1",
			Name:       "findme",
			Type:       ChannelWebhook,
			WebhookURL: "https://c.com",
		}
		if err := store.Create(ch); err != nil {
			t.Fatal(err)
		}
		got, ok := store.Get("get-1")
		if !ok {
			t.Fatal("expected to find channel")
		}
		if got.Name != "findme" {
			t.Errorf("Name = %q, want %q", got.Name, "findme")
		}
	})

	t.Run("get non-existent returns false", func(t *testing.T) {
		store, err := NewNotificationChannelStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		_, ok := store.Get("does-not-exist")
		if ok {
			t.Error("expected ok=false for non-existent channel")
		}
	})

	t.Run("update channel", func(t *testing.T) {
		store, err := NewNotificationChannelStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		ch := NotificationChannelConfig{
			ID:         "upd-1",
			Name:       "original",
			Type:       ChannelWebhook,
			WebhookURL: "https://d.com",
		}
		if err := store.Create(ch); err != nil {
			t.Fatal(err)
		}

		updated := NotificationChannelConfig{
			Name:       "updated-name",
			Type:       ChannelWebhook,
			WebhookURL: "https://e.com",
		}
		if err := store.Update("upd-1", updated); err != nil {
			t.Fatal(err)
		}

		got, ok := store.Get("upd-1")
		if !ok {
			t.Fatal("channel not found after update")
		}
		if got.Name != "updated-name" {
			t.Errorf("Name = %q, want %q", got.Name, "updated-name")
		}
		if got.WebhookURL != "https://e.com" {
			t.Errorf("WebhookURL = %q, want %q", got.WebhookURL, "https://e.com")
		}
	})

	t.Run("update preserves masked sensitive fields", func(t *testing.T) {
		store, err := NewNotificationChannelStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		ch := NotificationChannelConfig{
			ID:       "mask-1",
			Name:     "email-ch",
			Type:     ChannelEmail,
			Email:    "a@b.com",
			SMTPHost: "smtp.example.com",
			SMTPPass: "realsecret",
		}
		if err := store.Create(ch); err != nil {
			t.Fatal(err)
		}

		// Simulate the frontend sending back the masked value
		upd := NotificationChannelConfig{
			Name:     "email-ch",
			Type:     ChannelEmail,
			Email:    "a@b.com",
			SMTPHost: "smtp.example.com",
			SMTPPass: "••••••••",
		}
		if err := store.Update("mask-1", upd); err != nil {
			t.Fatal(err)
		}

		// Verify via ListRaw that original password was preserved
		raw := store.ListRaw()
		if len(raw) != 1 {
			t.Fatalf("expected 1 channel, got %d", len(raw))
		}
		if raw[0].SMTPPass != "realsecret" {
			t.Errorf("SMTPPass = %q, want original 'realsecret' preserved", raw[0].SMTPPass)
		}
	})

	t.Run("delete channel", func(t *testing.T) {
		store, err := NewNotificationChannelStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		ch := NotificationChannelConfig{
			ID:         "del-1",
			Name:       "to-delete",
			Type:       ChannelWebhook,
			WebhookURL: "https://f.com",
		}
		if err := store.Create(ch); err != nil {
			t.Fatal(err)
		}
		if err := store.Delete("del-1"); err != nil {
			t.Fatal(err)
		}
		list := store.List()
		if len(list) != 0 {
			t.Errorf("expected empty list after delete, got %d", len(list))
		}
	})

	t.Run("delete non-existent errors", func(t *testing.T) {
		store, err := NewNotificationChannelStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Delete("nope"); err == nil {
			t.Error("expected error deleting non-existent channel")
		}
	})

	t.Run("toggle enabled", func(t *testing.T) {
		store, err := NewNotificationChannelStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		ch := NotificationChannelConfig{
			ID:         "tog-1",
			Name:       "toggle-test",
			Type:       ChannelWebhook,
			Enabled:    true,
			WebhookURL: "https://g.com",
		}
		if err := store.Create(ch); err != nil {
			t.Fatal(err)
		}

		if err := store.ToggleEnabled("tog-1", false); err != nil {
			t.Fatal(err)
		}
		got, _ := store.Get("tog-1")
		if got.Enabled {
			t.Error("expected Enabled=false after toggle")
		}

		if err := store.ToggleEnabled("tog-1", true); err != nil {
			t.Fatal(err)
		}
		got, _ = store.Get("tog-1")
		if !got.Enabled {
			t.Error("expected Enabled=true after toggle")
		}
	})

	t.Run("persistence across store instances", func(t *testing.T) {
		dir := t.TempDir()
		store1, err := NewNotificationChannelStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		ch := NotificationChannelConfig{
			ID:         "persist-1",
			Name:       "persistent",
			Type:       ChannelWebhook,
			WebhookURL: "https://h.com",
		}
		if err := store1.Create(ch); err != nil {
			t.Fatal(err)
		}

		// Create a second store from the same directory
		store2, err := NewNotificationChannelStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		list := store2.List()
		if len(list) != 1 {
			t.Fatalf("expected 1 channel in new store, got %d", len(list))
		}
		if list[0].Name != "persistent" {
			t.Errorf("Name = %q, want %q", list[0].Name, "persistent")
		}
	})
}

// ---------------------------------------------------------------------------
// 4. NotificationDispatcher Filter Matching
// ---------------------------------------------------------------------------

func TestNotificationDispatcher_MatchesFilters(t *testing.T) {
	// Helper to create a dispatcher with a minimal store
	newDispatcher := func(t *testing.T) *NotificationDispatcher {
		t.Helper()
		store, err := NewNotificationChannelStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return NewNotificationDispatcher(store, nil, log.New(io.Discard, "", 0))
	}

	baseIncident := monitoring.Incident{
		ID:        "inc-1",
		CheckID:   "mysql-health",
		CheckName: "MySQL Health",
		Type:      "mysql",
		Severity:  "critical",
		Status:    "open",
		Message:   "Connection pool exhausted",
		StartedAt: time.Now(),
		Metadata:  map[string]string{"server": "prod-db-01"},
	}

	baseResult := &monitoring.CheckResult{
		CheckID: "mysql-health",
		Name:    "MySQL Health",
		Type:    "mysql",
		Server:  "prod-db-01",
		Tags:    []string{"database", "production"},
	}

	t.Run("no filters matches everything", func(t *testing.T) {
		d := newDispatcher(t)
		ch := NotificationChannelConfig{Enabled: true}
		if !d.matchesFilters(ch, baseIncident, baseResult) {
			t.Error("expected match with no filters")
		}
	})

	t.Run("severity filter matches", func(t *testing.T) {
		d := newDispatcher(t)
		ch := NotificationChannelConfig{Severities: []string{"critical"}}
		if !d.matchesFilters(ch, baseIncident, baseResult) {
			t.Error("expected match for critical severity")
		}
	})

	t.Run("severity filter rejects", func(t *testing.T) {
		d := newDispatcher(t)
		ch := NotificationChannelConfig{Severities: []string{"warning"}}
		if d.matchesFilters(ch, baseIncident, baseResult) {
			t.Error("expected rejection for severity mismatch")
		}
	})

	t.Run("checkID filter matches", func(t *testing.T) {
		d := newDispatcher(t)
		ch := NotificationChannelConfig{CheckIDs: []string{"mysql-health"}}
		if !d.matchesFilters(ch, baseIncident, baseResult) {
			t.Error("expected match for check ID")
		}
	})

	t.Run("checkID filter rejects", func(t *testing.T) {
		d := newDispatcher(t)
		ch := NotificationChannelConfig{CheckIDs: []string{"api-health"}}
		if d.matchesFilters(ch, baseIncident, baseResult) {
			t.Error("expected rejection for check ID mismatch")
		}
	})

	t.Run("checkType filter matches", func(t *testing.T) {
		d := newDispatcher(t)
		ch := NotificationChannelConfig{CheckTypes: []string{"api", "mysql"}}
		if !d.matchesFilters(ch, baseIncident, baseResult) {
			t.Error("expected match for check type")
		}
	})

	t.Run("checkType filter rejects", func(t *testing.T) {
		d := newDispatcher(t)
		ch := NotificationChannelConfig{CheckTypes: []string{"api"}}
		incident := baseIncident
		incident.Type = "mysql"
		if d.matchesFilters(ch, incident, baseResult) {
			t.Error("expected rejection for check type mismatch")
		}
	})

	t.Run("server filter with CheckResult", func(t *testing.T) {
		d := newDispatcher(t)
		ch := NotificationChannelConfig{Servers: []string{"prod-db-01"}}
		if !d.matchesFilters(ch, baseIncident, baseResult) {
			t.Error("expected match via CheckResult.Server")
		}
	})

	t.Run("server filter with incident metadata fallback", func(t *testing.T) {
		d := newDispatcher(t)
		ch := NotificationChannelConfig{Servers: []string{"prod-db-01"}}
		incident := baseIncident
		incident.Metadata = map[string]string{"server": "prod-db-01"}
		// nil result forces metadata fallback
		if !d.matchesFilters(ch, incident, nil) {
			t.Error("expected match via incident Metadata['server']")
		}
	})

	t.Run("server filter rejects when no match", func(t *testing.T) {
		d := newDispatcher(t)
		ch := NotificationChannelConfig{Servers: []string{"staging-db-01"}}
		incident := baseIncident
		incident.Metadata = map[string]string{"server": "prod-db-01"}
		if d.matchesFilters(ch, incident, nil) {
			t.Error("expected rejection for server mismatch")
		}
	})

	t.Run("tag filter matches", func(t *testing.T) {
		d := newDispatcher(t)
		ch := NotificationChannelConfig{Tags: []string{"production"}}
		if !d.matchesFilters(ch, baseIncident, baseResult) {
			t.Error("expected match for tag filter")
		}
	})

	t.Run("tag filter rejects when no tags match", func(t *testing.T) {
		d := newDispatcher(t)
		ch := NotificationChannelConfig{Tags: []string{"staging"}}
		if d.matchesFilters(ch, baseIncident, baseResult) {
			t.Error("expected rejection when no tags match")
		}
	})

	t.Run("tag filter rejects with nil result", func(t *testing.T) {
		d := newDispatcher(t)
		ch := NotificationChannelConfig{Tags: []string{"production"}}
		if d.matchesFilters(ch, baseIncident, nil) {
			t.Error("expected rejection for tag filter with nil result")
		}
	})

	t.Run("multiple filters combined AND logic", func(t *testing.T) {
		d := newDispatcher(t)
		ch := NotificationChannelConfig{
			Severities: []string{"critical"},
			CheckIDs:   []string{"mysql-health"},
			CheckTypes: []string{"mysql"},
			Servers:    []string{"prod-db-01"},
			Tags:       []string{"database"},
		}
		if !d.matchesFilters(ch, baseIncident, baseResult) {
			t.Error("expected match when all filters pass")
		}

		// Change one filter to not match
		ch.Severities = []string{"warning"}
		if d.matchesFilters(ch, baseIncident, baseResult) {
			t.Error("expected rejection when one filter fails (AND logic)")
		}
	})
}

// ---------------------------------------------------------------------------
// 5. Cooldown Logic
// ---------------------------------------------------------------------------

func TestNotificationDispatcher_Cooldown(t *testing.T) {
	newDispatcher := func(t *testing.T) *NotificationDispatcher {
		t.Helper()
		store, err := NewNotificationChannelStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		return NewNotificationDispatcher(store, nil, log.New(io.Discard, "", 0))
	}

	t.Run("no cooldown set returns false", func(t *testing.T) {
		d := newDispatcher(t)
		ch := NotificationChannelConfig{
			ID:              "ch-1",
			CooldownMinutes: 0,
		}
		if d.inCooldown(ch, "check-1") {
			t.Error("expected no cooldown when CooldownMinutes=0")
		}
	})

	t.Run("no prior send returns false", func(t *testing.T) {
		d := newDispatcher(t)
		ch := NotificationChannelConfig{
			ID:              "ch-1",
			CooldownMinutes: 5,
		}
		if d.inCooldown(ch, "check-1") {
			t.Error("expected no cooldown on first check")
		}
	})

	t.Run("within cooldown window returns true", func(t *testing.T) {
		d := newDispatcher(t)
		ch := NotificationChannelConfig{
			ID:              "ch-cool",
			CooldownMinutes: 60, // 60 minutes — we just recorded, so definitely within window
		}
		d.recordCooldown(ch, "check-1")
		if !d.inCooldown(ch, "check-1") {
			t.Error("expected in cooldown immediately after recording")
		}
	})

	t.Run("different check ID is not in cooldown", func(t *testing.T) {
		d := newDispatcher(t)
		ch := NotificationChannelConfig{
			ID:              "ch-cool",
			CooldownMinutes: 60,
		}
		d.recordCooldown(ch, "check-1")
		if d.inCooldown(ch, "check-2") {
			t.Error("expected no cooldown for different check ID")
		}
	})

	t.Run("CooldownMinutes=0 means never in cooldown after record", func(t *testing.T) {
		d := newDispatcher(t)
		ch := NotificationChannelConfig{
			ID:              "ch-nocool",
			CooldownMinutes: 0,
		}
		// recordCooldown is a no-op when CooldownMinutes <= 0
		d.recordCooldown(ch, "check-1")
		if d.inCooldown(ch, "check-1") {
			t.Error("expected no cooldown when CooldownMinutes=0")
		}
	})
}

// ---------------------------------------------------------------------------
// 6. Notification API Handler
// ---------------------------------------------------------------------------

func TestNotificationAPIHandler(t *testing.T) {
	setupServer := func(t *testing.T) (*httptest.Server, *NotificationChannelStore) {
		t.Helper()
		dir := t.TempDir()
		store, err := NewNotificationChannelStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		dispatcher := NewNotificationDispatcher(store, nil, log.New(io.Discard, "", 0))
		cfg := &monitoring.Config{Auth: monitoring.AuthConfig{Enabled: false}}
		handler := NewNotificationAPIHandler(store, dispatcher, cfg)

		mux := http.NewServeMux()
		handler.RegisterRoutes(mux)
		server := httptest.NewServer(mux)
		t.Cleanup(server.Close)
		return server, store
	}

	t.Run("GET returns empty list initially", func(t *testing.T) {
		srv, _ := setupServer(t)

		resp, err := http.Get(srv.URL + "/api/v1/notification-channels")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}

		var result monitoring.APIResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		// Data should be an empty array
		dataBytes, _ := json.Marshal(result.Data)
		var channels []NotificationChannelConfig
		if err := json.Unmarshal(dataBytes, &channels); err != nil {
			t.Fatal(err)
		}
		if len(channels) != 0 {
			t.Errorf("expected 0 channels, got %d", len(channels))
		}
	})

	t.Run("POST creates a channel", func(t *testing.T) {
		srv, _ := setupServer(t)

		body := `{
			"id": "test-ch-1",
			"name": "slack-ops",
			"type": "slack",
			"enabled": true,
			"webhookUrl": "https://hooks.slack.com/services/T/B/x"
		}`
		resp, err := http.Post(
			srv.URL+"/api/v1/notification-channels",
			"application/json",
			strings.NewReader(body),
		)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 201, body: %s", resp.StatusCode, string(respBody))
		}
	})

	t.Run("GET by ID returns channel", func(t *testing.T) {
		srv, store := setupServer(t)

		ch := NotificationChannelConfig{
			ID:         "get-ch-1",
			Name:       "webhook-test",
			Type:       ChannelWebhook,
			Enabled:    true,
			WebhookURL: "https://example.com/hook",
		}
		if err := store.Create(ch); err != nil {
			t.Fatal(err)
		}

		resp, err := http.Get(srv.URL + "/api/v1/notification-channels/get-ch-1")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}

		var result monitoring.APIResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatal(err)
		}
		dataBytes, _ := json.Marshal(result.Data)
		var got NotificationChannelConfig
		if err := json.Unmarshal(dataBytes, &got); err != nil {
			t.Fatal(err)
		}
		if got.Name != "webhook-test" {
			t.Errorf("Name = %q, want %q", got.Name, "webhook-test")
		}
	})

	t.Run("PUT updates channel", func(t *testing.T) {
		srv, store := setupServer(t)

		ch := NotificationChannelConfig{
			ID:         "put-ch-1",
			Name:       "before-update",
			Type:       ChannelWebhook,
			Enabled:    true,
			WebhookURL: "https://example.com/hook",
		}
		if err := store.Create(ch); err != nil {
			t.Fatal(err)
		}

		updateBody := `{
			"name": "after-update",
			"type": "webhook",
			"enabled": true,
			"webhookUrl": "https://example.com/hook-v2"
		}`
		req, err := http.NewRequest(
			http.MethodPut,
			srv.URL+"/api/v1/notification-channels/put-ch-1",
			strings.NewReader(updateBody),
		)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200, body: %s", resp.StatusCode, string(respBody))
		}

		// Verify update persisted
		got, ok := store.Get("put-ch-1")
		if !ok {
			t.Fatal("channel not found after update")
		}
		if got.Name != "after-update" {
			t.Errorf("Name = %q, want %q", got.Name, "after-update")
		}
	})

	t.Run("DELETE removes channel", func(t *testing.T) {
		srv, store := setupServer(t)

		ch := NotificationChannelConfig{
			ID:         "del-ch-1",
			Name:       "to-delete",
			Type:       ChannelWebhook,
			Enabled:    true,
			WebhookURL: "https://example.com/hook",
		}
		if err := store.Create(ch); err != nil {
			t.Fatal(err)
		}

		req, err := http.NewRequest(http.MethodDelete, srv.URL+"/api/v1/notification-channels/del-ch-1", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}

		_, ok := store.Get("del-ch-1")
		if ok {
			t.Error("channel still exists after delete")
		}
	})

	t.Run("POST toggle enabled", func(t *testing.T) {
		srv, store := setupServer(t)

		ch := NotificationChannelConfig{
			ID:         "tog-ch-1",
			Name:       "toggle-me",
			Type:       ChannelWebhook,
			Enabled:    true,
			WebhookURL: "https://example.com/hook",
		}
		if err := store.Create(ch); err != nil {
			t.Fatal(err)
		}

		toggleBody := `{"enabled": false}`
		resp, err := http.Post(
			srv.URL+"/api/v1/notification-channels/tog-ch-1/toggle",
			"application/json",
			strings.NewReader(toggleBody),
		)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			t.Fatalf("status = %d, want 200, body: %s", resp.StatusCode, string(respBody))
		}

		got, ok := store.Get("tog-ch-1")
		if !ok {
			t.Fatal("channel not found after toggle")
		}
		if got.Enabled {
			t.Error("expected Enabled=false after toggle")
		}
	})

	t.Run("POST invalid JSON returns 400", func(t *testing.T) {
		srv, _ := setupServer(t)

		resp, err := http.Post(
			srv.URL+"/api/v1/notification-channels",
			"application/json",
			strings.NewReader(`{invalid json`),
		)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("POST missing required fields returns 400", func(t *testing.T) {
		srv, _ := setupServer(t)

		// Valid JSON but missing 'name'
		body := `{"type": "slack", "webhookUrl": "https://x.com"}`
		resp, err := http.Post(
			srv.URL+"/api/v1/notification-channels",
			"application/json",
			strings.NewReader(body),
		)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("GET non-existent channel returns 404", func(t *testing.T) {
		srv, _ := setupServer(t)

		resp, err := http.Get(srv.URL + "/api/v1/notification-channels/does-not-exist")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// apiResponseData is a helper to unmarshal the nested APIResponse.Data field.
func apiResponseData(t *testing.T, body io.Reader, target interface{}) {
	t.Helper()
	var resp monitoring.APIResponse
	if err := json.NewDecoder(body).Decode(&resp); err != nil {
		t.Fatalf("decode APIResponse: %v", err)
	}
	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("re-marshal data: %v", err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatalf("unmarshal data into target: %v", err)
	}
}

// suppress unused import warning for fmt
var _ = fmt.Sprintf
