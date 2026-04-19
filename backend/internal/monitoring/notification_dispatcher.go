package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"strings"
	"sync"
	"time"
)

// NotificationPayload is the structured data sent to channels.
type NotificationPayload struct {
	IncidentID string `json:"incidentId"`
	CheckID    string `json:"checkId"`
	CheckName  string `json:"checkName"`
	CheckType  string `json:"type"`
	Server     string `json:"server,omitempty"`
	Severity   string `json:"severity"`
	Status     string `json:"status"` // open, resolved
	Message    string `json:"message"`
	StartedAt  string `json:"startedAt"`
	ResolvedAt string `json:"resolvedAt,omitempty"`
}

// NotificationDispatcher evaluates channel filters and dispatches notifications.
type NotificationDispatcher struct {
	channelStore *NotificationChannelStore
	outbox       NotificationOutboxRepository
	logger       *log.Logger
	httpClient   *http.Client

	// Track cooldowns: channelID:checkID → last sent time
	cooldowns map[string]time.Time
	mu        sync.Mutex
}

// NewNotificationDispatcher creates a dispatcher wired to the channel store.
func NewNotificationDispatcher(
	channelStore *NotificationChannelStore,
	outbox NotificationOutboxRepository,
	logger *log.Logger,
) *NotificationDispatcher {
	if logger == nil {
		logger = log.Default()
	}
	return &NotificationDispatcher{
		channelStore: channelStore,
		outbox:       outbox,
		logger:       logger,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		cooldowns: make(map[string]time.Time),
	}
}

// NotifyIncident evaluates all channels and sends notifications for matching ones.
// checkResult is optional — when available, provides extra filter context (server, type, tags).
func (d *NotificationDispatcher) NotifyIncident(incident Incident, checkResult *CheckResult) {
	channels := d.channelStore.ListRaw()
	if len(channels) == 0 {
		return
	}

	payload := buildPayload(incident, "open")

	for _, ch := range channels {
		if !ch.Enabled {
			continue
		}
		if !d.matchesFilters(ch, incident, checkResult) {
			continue
		}
		if d.inCooldown(ch, incident.CheckID) {
			d.logger.Printf("notification: channel %q in cooldown for check %s", ch.Name, incident.CheckID)
			continue
		}

		// Send async — don't block incident creation
		go d.sendToChannel(ch, payload, incident.ID)
		d.recordCooldown(ch, incident.CheckID)
	}
}

// NotifyResolved sends resolution notifications to channels with notifyOnResolve enabled.
func (d *NotificationDispatcher) NotifyResolved(incident Incident, checkResult *CheckResult) {
	channels := d.channelStore.ListRaw()

	payload := buildPayload(incident, "resolved")
	if incident.ResolvedAt != nil {
		payload.ResolvedAt = incident.ResolvedAt.Format(time.RFC3339)
	}

	for _, ch := range channels {
		if !ch.Enabled || !ch.NotifyOnResolve {
			continue
		}
		if !d.matchesFilters(ch, incident, checkResult) {
			continue
		}
		go d.sendToChannel(ch, payload, incident.ID)
	}
}

// matchesFilters checks if an incident matches a channel's smart filters.
func (d *NotificationDispatcher) matchesFilters(ch NotificationChannelConfig, incident Incident, result *CheckResult) bool {
	// Severity filter
	if len(ch.Severities) > 0 && !containsStr(ch.Severities, incident.Severity) {
		return false
	}

	// Check ID filter
	if len(ch.CheckIDs) > 0 && !containsStr(ch.CheckIDs, incident.CheckID) {
		return false
	}

	// Check type filter
	if len(ch.CheckTypes) > 0 && !containsStr(ch.CheckTypes, incident.Type) {
		return false
	}

	// Server filter — need check result for this
	if len(ch.Servers) > 0 {
		if result == nil || !containsStr(ch.Servers, result.Server) {
			// Also check incident metadata for server info
			if srv, ok := incident.Metadata["server"]; ok {
				if !containsStr(ch.Servers, srv) {
					return false
				}
			} else if result == nil {
				return false
			}
		}
	}

	// Tag filter — check must have at least one matching tag
	if len(ch.Tags) > 0 {
		if result == nil {
			return false
		}
		found := false
		for _, tag := range ch.Tags {
			if containsStr(result.Tags, tag) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func (d *NotificationDispatcher) inCooldown(ch NotificationChannelConfig, checkID string) bool {
	if ch.CooldownMinutes <= 0 {
		return false
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	key := fmt.Sprintf("%s:%s", ch.ID, checkID)
	lastSent, ok := d.cooldowns[key]
	if !ok {
		return false
	}
	return time.Since(lastSent) < time.Duration(ch.CooldownMinutes)*time.Minute
}

func (d *NotificationDispatcher) recordCooldown(ch NotificationChannelConfig, checkID string) {
	if ch.CooldownMinutes <= 0 {
		return
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	key := fmt.Sprintf("%s:%s", ch.ID, checkID)
	d.cooldowns[key] = time.Now()
}

// sendToChannel dispatches the notification to the specific channel type.
func (d *NotificationDispatcher) sendToChannel(ch NotificationChannelConfig, payload NotificationPayload, incidentID string) {
	var err error

	switch ch.Type {
	case ChannelSlack:
		err = d.sendSlack(ch, payload)
	case ChannelDiscord:
		err = d.sendDiscord(ch, payload)
	case ChannelWebhook:
		err = d.sendWebhook(ch, payload)
	case ChannelEmail:
		err = d.sendEmail(ch, payload)
	case ChannelTelegram:
		err = d.sendTelegram(ch, payload)
	case ChannelPagerDuty:
		err = d.sendPagerDuty(ch, payload)
	default:
		err = fmt.Errorf("unsupported channel type: %s", ch.Type)
	}

	// Record in outbox for audit trail
	if d.outbox != nil {
		payloadJSON, _ := json.Marshal(payload)
		evt := NotificationEvent{
			IncidentID:  incidentID,
			Channel:     fmt.Sprintf("%s:%s", ch.Type, ch.Name),
			PayloadJSON: string(payloadJSON),
		}
		if err != nil {
			evt.LastError = err.Error()
		}
		if enqErr := d.outbox.Enqueue(evt); enqErr != nil {
			d.logger.Printf("notification: failed to record in outbox: %v", enqErr)
		}
		if err == nil {
			// Mark as sent immediately since we already delivered
			if evt.NotificationID != "" {
				_ = d.outbox.MarkSent(evt.NotificationID)
			}
		}
	}

	if err != nil {
		d.logger.Printf("notification: failed to send to %s channel %q: %v", ch.Type, ch.Name, err)
	} else {
		d.logger.Printf("notification: sent to %s channel %q for incident %s", ch.Type, ch.Name, incidentID)
	}
}

// --- Slack ---

func (d *NotificationDispatcher) sendSlack(ch NotificationChannelConfig, p NotificationPayload) error {
	color := "#36a64f" // green
	if p.Severity == "critical" {
		color = "#e01e5a"
	} else if p.Severity == "warning" {
		color = "#ecb22e"
	}
	if p.Status == "resolved" {
		color = "#36a64f"
	}

	statusEmoji := "🔴"
	if p.Status == "resolved" {
		statusEmoji = "✅"
	} else if p.Severity == "warning" {
		statusEmoji = "🟡"
	}

	title := fmt.Sprintf("%s %s — %s", statusEmoji, strings.ToUpper(p.Status), p.CheckName)

	slackBody := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color":  color,
				"title":  title,
				"text":   p.Message,
				"footer": "HealthOps",
				"ts":     time.Now().Unix(),
				"fields": []map[string]string{
					{"title": "Check", "value": p.CheckName, "short": "true"},
					{"title": "Severity", "value": strings.ToUpper(p.Severity), "short": "true"},
					{"title": "Type", "value": p.CheckType, "short": "true"},
					{"title": "Server", "value": p.Server, "short": "true"},
				},
			},
		},
	}

	return d.postJSON(ch.WebhookURL, slackBody, nil)
}

// --- Discord ---

func (d *NotificationDispatcher) sendDiscord(ch NotificationChannelConfig, p NotificationPayload) error {
	color := 0x36a64f
	if p.Severity == "critical" {
		color = 0xe01e5a
	} else if p.Severity == "warning" {
		color = 0xecb22e
	}
	if p.Status == "resolved" {
		color = 0x36a64f
	}

	discordBody := map[string]interface{}{
		"embeds": []map[string]interface{}{
			{
				"title":       fmt.Sprintf("%s — %s", strings.ToUpper(p.Status), p.CheckName),
				"description": p.Message,
				"color":       color,
				"fields": []map[string]interface{}{
					{"name": "Severity", "value": strings.ToUpper(p.Severity), "inline": true},
					{"name": "Type", "value": p.CheckType, "inline": true},
					{"name": "Server", "value": p.Server, "inline": true},
				},
				"footer":    map[string]string{"text": "HealthOps"},
				"timestamp": time.Now().Format(time.RFC3339),
			},
		},
	}

	return d.postJSON(ch.WebhookURL, discordBody, nil)
}

// --- Generic Webhook ---

func (d *NotificationDispatcher) sendWebhook(ch NotificationChannelConfig, p NotificationPayload) error {
	return d.postJSON(ch.WebhookURL, p, ch.Headers)
}

// --- Email (SMTP) ---

func (d *NotificationDispatcher) sendEmail(ch NotificationChannelConfig, p NotificationPayload) error {
	subject := fmt.Sprintf("[HealthOps] %s — %s (%s)", strings.ToUpper(p.Status), p.CheckName, strings.ToUpper(p.Severity))

	body := fmt.Sprintf(
		"Incident: %s\nCheck: %s (%s)\nSeverity: %s\nServer: %s\nStatus: %s\nStarted: %s\n\n%s",
		p.IncidentID, p.CheckName, p.CheckType,
		strings.ToUpper(p.Severity), p.Server,
		strings.ToUpper(p.Status), p.StartedAt,
		p.Message,
	)
	if p.ResolvedAt != "" {
		body += fmt.Sprintf("\nResolved: %s", p.ResolvedAt)
	}

	from := ch.FromEmail
	if from == "" {
		from = ch.SMTPUser
	}

	recipients := strings.Split(ch.Email, ",")
	for i := range recipients {
		recipients[i] = strings.TrimSpace(recipients[i])
	}

	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		from, strings.Join(recipients, ","), subject, body,
	)

	addr := fmt.Sprintf("%s:%d", ch.SMTPHost, ch.SMTPPort)
	var auth smtp.Auth
	if ch.SMTPUser != "" {
		auth = smtp.PlainAuth("", ch.SMTPUser, ch.SMTPPass, ch.SMTPHost)
	}

	return smtp.SendMail(addr, auth, from, recipients, []byte(msg))
}

// --- Telegram ---

func (d *NotificationDispatcher) sendTelegram(ch NotificationChannelConfig, p NotificationPayload) error {
	statusEmoji := "🔴"
	if p.Status == "resolved" {
		statusEmoji = "✅"
	} else if p.Severity == "warning" {
		statusEmoji = "🟡"
	}

	text := fmt.Sprintf(
		"%s *%s — %s*\n\n*Severity:* %s\n*Type:* %s\n*Server:* %s\n\n%s",
		statusEmoji, strings.ToUpper(p.Status), escapeMarkdown(p.CheckName),
		strings.ToUpper(p.Severity), p.CheckType, p.Server,
		escapeMarkdown(p.Message),
	)

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", ch.BotToken)
	body := map[string]interface{}{
		"chat_id":    ch.ChatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	return d.postJSON(url, body, nil)
}

// --- PagerDuty ---

func (d *NotificationDispatcher) sendPagerDuty(ch NotificationChannelConfig, p NotificationPayload) error {
	eventAction := "trigger"
	if p.Status == "resolved" {
		eventAction = "resolve"
	}

	pdBody := map[string]interface{}{
		"routing_key":  ch.RoutingKey,
		"event_action": eventAction,
		"dedup_key":    p.IncidentID,
		"payload": map[string]interface{}{
			"summary":   fmt.Sprintf("%s — %s (%s)", p.CheckName, p.Message, strings.ToUpper(p.Severity)),
			"severity":  mapPDSeverity(p.Severity),
			"source":    p.Server,
			"component": p.CheckName,
			"group":     p.CheckType,
			"custom_details": map[string]string{
				"check_id":    p.CheckID,
				"incident_id": p.IncidentID,
				"started_at":  p.StartedAt,
			},
		},
	}

	return d.postJSON("https://events.pagerduty.com/v2/enqueue", pdBody, nil)
}

// --- Helpers ---

func (d *NotificationDispatcher) postJSON(url string, body interface{}, headers map[string]string) error {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}
	return nil
}

func buildPayload(incident Incident, status string) NotificationPayload {
	return NotificationPayload{
		IncidentID: incident.ID,
		CheckID:    incident.CheckID,
		CheckName:  incident.CheckName,
		CheckType:  incident.Type,
		Server:     incident.Metadata["server"],
		Severity:   incident.Severity,
		Status:     status,
		Message:    incident.Message,
		StartedAt:  incident.StartedAt.Format(time.RFC3339),
	}
}

func containsStr(slice []string, s string) bool {
	for _, item := range slice {
		if strings.EqualFold(item, s) {
			return true
		}
	}
	return false
}

func escapeMarkdown(s string) string {
	replacer := strings.NewReplacer(
		"_", "\\_", "*", "\\*", "[", "\\[", "]", "\\]",
		"(", "\\(", ")", "\\)", "~", "\\~", "`", "\\`",
		">", "\\>", "#", "\\#", "+", "\\+", "-", "\\-",
		"=", "\\=", "|", "\\|", "{", "\\{", "}", "\\}",
		".", "\\.", "!", "\\!",
	)
	return replacer.Replace(s)
}

func mapPDSeverity(severity string) string {
	switch severity {
	case "critical":
		return "critical"
	case "warning":
		return "warning"
	default:
		return "info"
	}
}

// TestChannel sends a test notification to verify channel configuration.
func (d *NotificationDispatcher) TestChannel(ch NotificationChannelConfig) error {
	payload := NotificationPayload{
		IncidentID: "test-" + fmt.Sprintf("%d", time.Now().Unix()),
		CheckID:    "test-check",
		CheckName:  "Test Check",
		CheckType:  "api",
		Server:     "test-server",
		Severity:   "warning",
		Status:     "open",
		Message:    "This is a test notification from HealthOps to verify your channel configuration.",
		StartedAt:  time.Now().Format(time.RFC3339),
	}

	var err error
	switch ch.Type {
	case ChannelSlack:
		err = d.sendSlack(ch, payload)
	case ChannelDiscord:
		err = d.sendDiscord(ch, payload)
	case ChannelWebhook:
		err = d.sendWebhook(ch, payload)
	case ChannelEmail:
		err = d.sendEmail(ch, payload)
	case ChannelTelegram:
		err = d.sendTelegram(ch, payload)
	case ChannelPagerDuty:
		err = d.sendPagerDuty(ch, payload)
	default:
		err = fmt.Errorf("unsupported channel type: %s", ch.Type)
	}

	return err
}
