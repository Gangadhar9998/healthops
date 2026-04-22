package repositories

import (
	"context"
	"fmt"
	"strings"

	"medics-health-check/backend/internal/monitoring/notify"
)

// ChannelStoreAdapter adapts MongoChannelRepository to notify.ChannelStore.
type ChannelStoreAdapter struct {
	repo *MongoChannelRepository
}

// NewChannelStoreAdapter creates a Mongo-backed notification channel store adapter.
func NewChannelStoreAdapter(repo *MongoChannelRepository) *ChannelStoreAdapter {
	if repo == nil {
		panic("mongo channel repository cannot be nil")
	}
	return &ChannelStoreAdapter{repo: repo}
}

func (a *ChannelStoreAdapter) List() []notify.NotificationChannelConfig {
	channels, err := a.repo.List(context.Background())
	if err != nil {
		return nil
	}

	out := make([]notify.NotificationChannelConfig, len(channels))
	for i, channel := range channels {
		cfg := a.toNotify(channel)
		out[i] = cfg.SafeView()
	}
	return out
}

func (a *ChannelStoreAdapter) ListRaw() []notify.NotificationChannelConfig {
	channels, err := a.repo.List(context.Background())
	if err != nil {
		return nil
	}

	out := make([]notify.NotificationChannelConfig, len(channels))
	for i, channel := range channels {
		out[i] = a.toNotify(channel)
	}
	return out
}

func (a *ChannelStoreAdapter) Get(id string) (notify.NotificationChannelConfig, bool) {
	channel, err := a.repo.Get(context.Background(), id)
	if err != nil {
		return notify.NotificationChannelConfig{}, false
	}
	cfg := a.toNotify(*channel)
	return cfg.SafeView(), true
}

func (a *ChannelStoreAdapter) Create(ch notify.NotificationChannelConfig) error {
	repoChannel := a.toRepository(ch)
	return a.repo.Create(context.Background(), &repoChannel)
}

func (a *ChannelStoreAdapter) Update(id string, ch notify.NotificationChannelConfig) error {
	existing, err := a.repo.Get(context.Background(), id)
	if err != nil {
		return err
	}

	merged := a.toNotify(*existing)
	merged.Name = ch.Name
	merged.Type = ch.Type
	merged.Enabled = ch.Enabled
	merged.WebhookURL = ch.WebhookURL
	merged.Email = ch.Email
	merged.SMTPHost = ch.SMTPHost
	merged.SMTPPort = ch.SMTPPort
	merged.SMTPUser = ch.SMTPUser
	merged.FromEmail = ch.FromEmail
	merged.ChatID = ch.ChatID
	merged.Severities = append([]string(nil), ch.Severities...)
	merged.CheckIDs = append([]string(nil), ch.CheckIDs...)
	merged.CheckTypes = append([]string(nil), ch.CheckTypes...)
	merged.Servers = append([]string(nil), ch.Servers...)
	merged.Tags = append([]string(nil), ch.Tags...)
	merged.CooldownMinutes = ch.CooldownMinutes
	merged.MinConsecutiveFailures = ch.MinConsecutiveFailures
	merged.NotifyOnResolve = ch.NotifyOnResolve
	merged.Headers = cloneStringMap(ch.Headers)
	merged.CreatedAt = existing.CreatedAt

	if ch.SMTPPass != "" && ch.SMTPPass != "••••••••" {
		merged.SMTPPass = ch.SMTPPass
	}
	if ch.SMTPPass == "" {
		merged.SMTPPass = ""
	}
	if ch.BotToken != "" && !isMaskedSecret(ch.BotToken) {
		merged.BotToken = ch.BotToken
	}
	if ch.BotToken == "" {
		merged.BotToken = ""
	}
	if ch.RoutingKey != "" && !isMaskedSecret(ch.RoutingKey) {
		merged.RoutingKey = ch.RoutingKey
	}
	if ch.RoutingKey == "" {
		merged.RoutingKey = ""
	}

	repoChannel := a.toRepository(merged)
	repoChannel.CreatedAt = existing.CreatedAt
	return a.repo.Update(context.Background(), id, &repoChannel)
}

func (a *ChannelStoreAdapter) Delete(id string) error {
	return a.repo.Delete(context.Background(), id)
}

func (a *ChannelStoreAdapter) ToggleEnabled(id string, enabled bool) error {
	existing, err := a.repo.Get(context.Background(), id)
	if err != nil {
		return err
	}
	existing.Enabled = enabled
	return a.repo.Update(context.Background(), id, existing)
}

func (a *ChannelStoreAdapter) toNotify(channel NotificationChannel) notify.NotificationChannelConfig {
	return notify.NotificationChannelConfig{
		ID:                     channel.ID,
		Name:                   channel.Name,
		Type:                   notify.ChannelType(channel.Type),
		Enabled:                channel.Enabled,
		WebhookURL:             channel.WebhookURL,
		Email:                  channel.Email,
		SMTPHost:               channel.SMTPHost,
		SMTPPort:               channel.SMTPPort,
		SMTPUser:               channel.SMTPUser,
		SMTPPass:               channel.SMTPPass,
		FromEmail:              channel.FromEmail,
		BotToken:               channel.BotToken,
		ChatID:                 channel.ChatID,
		RoutingKey:             channel.RoutingKey,
		Severities:             append([]string(nil), channel.Severities...),
		CheckIDs:               append([]string(nil), channel.CheckIDs...),
		CheckTypes:             append([]string(nil), channel.CheckTypes...),
		Servers:                append([]string(nil), channel.Servers...),
		Tags:                   append([]string(nil), channel.Tags...),
		CooldownMinutes:        channel.CooldownMinutes,
		MinConsecutiveFailures: channel.MinConsecutiveFailures,
		NotifyOnResolve:        channel.NotifyOnResolve,
		Headers:                cloneStringMap(channel.Headers),
		CreatedAt:              channel.CreatedAt,
		UpdatedAt:              channel.UpdatedAt,
	}
}

func (a *ChannelStoreAdapter) toRepository(channel notify.NotificationChannelConfig) NotificationChannel {
	return NotificationChannel{
		ID:                     channel.ID,
		Name:                   channel.Name,
		Type:                   string(channel.Type),
		Enabled:                channel.Enabled,
		WebhookURL:             channel.WebhookURL,
		Email:                  channel.Email,
		SMTPHost:               channel.SMTPHost,
		SMTPPort:               channel.SMTPPort,
		SMTPUser:               channel.SMTPUser,
		SMTPPass:               channel.SMTPPass,
		FromEmail:              channel.FromEmail,
		BotToken:               channel.BotToken,
		ChatID:                 channel.ChatID,
		RoutingKey:             channel.RoutingKey,
		Severities:             append([]string(nil), channel.Severities...),
		CheckIDs:               append([]string(nil), channel.CheckIDs...),
		CheckTypes:             append([]string(nil), channel.CheckTypes...),
		Servers:                append([]string(nil), channel.Servers...),
		Tags:                   append([]string(nil), channel.Tags...),
		CooldownMinutes:        channel.CooldownMinutes,
		MinConsecutiveFailures: channel.MinConsecutiveFailures,
		NotifyOnResolve:        channel.NotifyOnResolve,
		Headers:                cloneStringMap(channel.Headers),
		CreatedAt:              channel.CreatedAt,
		UpdatedAt:              channel.UpdatedAt,
	}
}

func isMaskedSecret(value string) bool {
	return strings.Contains(value, "••••")
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

var _ notify.ChannelStore = (*ChannelStoreAdapter)(nil)

func (a *ChannelStoreAdapter) String() string {
	return fmt.Sprintf("ChannelStoreAdapter(%T)", a.repo)
}
