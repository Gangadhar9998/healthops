package repositories

import (
	"reflect"
	"testing"
	"time"

	"medics-health-check/backend/internal/monitoring/notify"
)

func TestChannelStoreAdapterRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Round(time.Second)
	src := notify.NotificationChannelConfig{
		ID:                     "ops-email",
		Name:                   "Ops Email",
		Type:                   notify.ChannelEmail,
		Enabled:                true,
		WebhookURL:             "https://example.com/hook",
		Email:                  "ops@example.com",
		SMTPHost:               "smtp.example.com",
		SMTPPort:               587,
		SMTPUser:               "smtp-user",
		SMTPPass:               "smtp-pass",
		FromEmail:              "healthops@example.com",
		BotToken:               "bot-token",
		ChatID:                 "chat-id",
		RoutingKey:             "routing-key",
		Severities:             []string{"critical", "warning"},
		CheckIDs:               []string{"check-1"},
		CheckTypes:             []string{"api"},
		Servers:                []string{"prod"},
		Tags:                   []string{"team:ops"},
		CooldownMinutes:        10,
		MinConsecutiveFailures: 2,
		NotifyOnResolve:        true,
		Headers:                map[string]string{"X-Test": "true"},
		CreatedAt:              now,
		UpdatedAt:              now,
	}

	adapter := &ChannelStoreAdapter{}
	repoChannel := adapter.toRepository(src)
	got := adapter.toNotify(repoChannel)

	if got.ID != src.ID || got.Name != src.Name || got.Type != src.Type || got.SMTPPass != src.SMTPPass {
		t.Fatalf("roundtrip lost core fields: got %+v", got)
	}
	if got.CooldownMinutes != src.CooldownMinutes || got.MinConsecutiveFailures != src.MinConsecutiveFailures {
		t.Fatalf("roundtrip lost numeric fields: got %+v", got)
	}
	if got.NotifyOnResolve != src.NotifyOnResolve {
		t.Fatalf("roundtrip lost notifyOnResolve flag")
	}
	if !reflect.DeepEqual(got.Headers, src.Headers) {
		t.Fatalf("headers mismatch: got %#v want %#v", got.Headers, src.Headers)
	}
	if !reflect.DeepEqual(got.Severities, src.Severities) || !reflect.DeepEqual(got.Tags, src.Tags) {
		t.Fatalf("slice fields mismatch: got %+v want %+v", got, src)
	}
}
