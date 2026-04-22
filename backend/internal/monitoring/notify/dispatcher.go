package notify

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

	"medics-health-check/backend/internal/monitoring"
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
	channelStore ChannelStore
	outbox       NotificationOutboxRepository
	logger       *log.Logger
	httpClient   *http.Client
	dashboardURL string // optional: base URL for dashboard links in emails

	// Track cooldowns: channelID:checkID → last sent time
	cooldowns map[string]time.Time
	// Track notified incidents to prevent duplicates: incidentID:channelID
	notified map[string]bool
	mu       sync.Mutex

	// Batching: collect incidents within a window and send consolidated notifications
	batchWindow  time.Duration
	batchTimer   *time.Timer
	pendingBatch []pendingNotification
	batchMu      sync.Mutex
}

// pendingNotification holds a buffered incident waiting to be batched.
type pendingNotification struct {
	incident        monitoring.Incident
	checkResult     *monitoring.CheckResult
	checkChannelIDs []string
}

// NewNotificationDispatcher creates a dispatcher wired to the channel store.
func NewNotificationDispatcher(
	channelStore ChannelStore,
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
		cooldowns:   make(map[string]time.Time),
		notified:    make(map[string]bool),
		batchWindow: 5 * time.Second,
	}
}

// Stop flushes any pending batch and stops the batch timer.
// Call this on graceful shutdown to ensure no notifications are lost.
func (d *NotificationDispatcher) Stop() {
	d.batchMu.Lock()
	if d.batchTimer != nil {
		d.batchTimer.Stop()
		d.batchTimer = nil
	}
	d.batchMu.Unlock()

	// Flush remaining pending notifications synchronously
	d.flushBatch()
}

// SetDashboardURL sets the base URL used for dashboard links in email notifications.
func (d *NotificationDispatcher) SetDashboardURL(url string) {
	d.dashboardURL = url
}

// NotifyIncident buffers the incident for batched notification.
// When multiple checks fail within the same cycle (batchWindow), a single
// consolidated digest is sent per channel instead of N separate messages.
// checkChannelIDs is optional — when provided, these channel IDs are always included.
func (d *NotificationDispatcher) NotifyIncident(incident monitoring.Incident, checkResult *monitoring.CheckResult, checkChannelIDs ...string) {
	channels := d.channelStore.ListRaw()
	if len(channels) == 0 {
		return
	}

	d.batchMu.Lock()
	defer d.batchMu.Unlock()

	d.pendingBatch = append(d.pendingBatch, pendingNotification{
		incident:        incident,
		checkResult:     checkResult,
		checkChannelIDs: checkChannelIDs,
	})

	// Reset the batch timer — flush after batchWindow of silence
	if d.batchTimer != nil {
		d.batchTimer.Stop()
	}
	d.batchTimer = time.AfterFunc(d.batchWindow, d.flushBatch)
}

// flushBatch sends all buffered notifications. Single incident → normal message.
// Multiple incidents → consolidated digest per channel.
func (d *NotificationDispatcher) flushBatch() {
	d.batchMu.Lock()
	batch := d.pendingBatch
	d.pendingBatch = nil
	d.batchTimer = nil
	d.batchMu.Unlock()

	if len(batch) == 0 {
		return
	}

	channels := d.channelStore.ListRaw()
	if len(channels) == 0 {
		return
	}

	// For each channel, determine which incidents in this batch match it.
	type channelBatch struct {
		channel   NotificationChannelConfig
		payloads  []NotificationPayload
		incidents []monitoring.Incident
	}

	channelBatches := make(map[string]*channelBatch)

	for _, pn := range batch {
		explicitIDs := make(map[string]bool, len(pn.checkChannelIDs))
		for _, id := range pn.checkChannelIDs {
			explicitIDs[id] = true
		}

		payload := buildPayload(pn.incident, "open")

		for _, ch := range channels {
			if !ch.Enabled {
				continue
			}
			if !explicitIDs[ch.ID] && !d.matchesFilters(ch, pn.incident, pn.checkResult) {
				continue
			}
			if d.inCooldown(ch, pn.incident.CheckID) {
				d.logger.Printf("notification: channel %q in cooldown for check %s", ch.Name, pn.incident.CheckID)
				continue
			}
			if d.alreadyNotified(pn.incident.ID, ch.ID) {
				continue
			}

			cb, ok := channelBatches[ch.ID]
			if !ok {
				cb = &channelBatch{channel: ch}
				channelBatches[ch.ID] = cb
			}
			cb.payloads = append(cb.payloads, payload)
			cb.incidents = append(cb.incidents, pn.incident)

			d.recordCooldown(ch, pn.incident.CheckID)
			d.markNotified(pn.incident.ID, ch.ID)
		}
	}

	// Dispatch per channel — single or digest
	for _, cb := range channelBatches {
		go func(cb *channelBatch) {
			defer func() {
				if r := recover(); r != nil {
					d.logger.Printf("notification: panic in batch send for %q: %v", cb.channel.Name, r)
				}
			}()
			if len(cb.payloads) == 1 {
				d.sendToChannel(cb.channel, cb.payloads[0], cb.incidents[0].ID)
			} else {
				d.sendDigest(cb.channel, cb.payloads)
			}
		}(cb)
	}
}

// NotifyResolved sends resolution notifications to channels with notifyOnResolve enabled.
func (d *NotificationDispatcher) NotifyResolved(incident monitoring.Incident, checkResult *monitoring.CheckResult, checkChannelIDs ...string) {
	// Clear dedup tracking so reopened incidents can trigger fresh notifications
	d.ClearIncident(incident.ID)

	channels := d.channelStore.ListRaw()

	// Build a set of explicitly assigned channel IDs from the check config
	explicitIDs := make(map[string]bool, len(checkChannelIDs))
	for _, id := range checkChannelIDs {
		explicitIDs[id] = true
	}

	payload := buildPayload(incident, "resolved")
	if incident.ResolvedAt != nil {
		payload.ResolvedAt = incident.ResolvedAt.Format(time.RFC3339)
	}

	for _, ch := range channels {
		if !ch.Enabled || !ch.NotifyOnResolve {
			continue
		}
		if !explicitIDs[ch.ID] && !d.matchesFilters(ch, incident, checkResult) {
			continue
		}
		go func(ch NotificationChannelConfig, p NotificationPayload, incID string) {
			defer func() {
				if r := recover(); r != nil {
					d.logger.Printf("notification: panic in sendToChannel for %q: %v", ch.Name, r)
				}
			}()
			d.sendToChannel(ch, p, incID)
		}(ch, payload, incident.ID)
	}
}

// matchesFilters checks if an incident matches a channel's smart filters.
func (d *NotificationDispatcher) matchesFilters(ch NotificationChannelConfig, incident monitoring.Incident, result *monitoring.CheckResult) bool {
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

func (d *NotificationDispatcher) alreadyNotified(incidentID, channelID string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.notified[incidentID+":"+channelID]
}

func (d *NotificationDispatcher) markNotified(incidentID, channelID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.notified[incidentID+":"+channelID] = true
}

// ClearIncident removes dedup tracking for a resolved incident so re-opened incidents can notify again.
func (d *NotificationDispatcher) ClearIncident(incidentID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for key := range d.notified {
		if strings.HasPrefix(key, incidentID+":") {
			delete(d.notified, key)
		}
	}
}

// CleanupStaleTracking prunes cooldown and dedup entries older than 24 hours.
func (d *NotificationDispatcher) CleanupStaleTracking() {
	d.mu.Lock()
	defer d.mu.Unlock()

	cutoff := time.Now().Add(-24 * time.Hour)
	for key, lastSent := range d.cooldowns {
		if lastSent.Before(cutoff) {
			delete(d.cooldowns, key)
		}
	}
	// notified map: clear entries for old incidents (best-effort, uses same cutoff)
	// Since notified is bool-only, we clear all entries periodically
	if len(d.notified) > 10000 {
		d.notified = make(map[string]bool)
	}
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
		evt := monitoring.NotificationEvent{
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

// sendDigest sends a consolidated notification for multiple incidents to a single channel.
func (d *NotificationDispatcher) sendDigest(ch NotificationChannelConfig, payloads []NotificationPayload) {
	var err error

	switch ch.Type {
	case ChannelSlack:
		err = d.sendSlackDigest(ch, payloads)
	case ChannelDiscord:
		err = d.sendDiscordDigest(ch, payloads)
	case ChannelWebhook:
		err = d.sendWebhookDigest(ch, payloads)
	case ChannelEmail:
		err = d.sendEmailDigest(ch, payloads)
	case ChannelTelegram:
		err = d.sendTelegramDigest(ch, payloads)
	case ChannelPagerDuty:
		// PagerDuty requires one event per incident for proper dedup_key tracking
		for _, p := range payloads {
			if pErr := d.sendPagerDuty(ch, p); pErr != nil {
				d.logger.Printf("notification: failed to send pagerduty for %s: %v", p.IncidentID, pErr)
			}
		}
		return
	default:
		err = fmt.Errorf("unsupported channel type: %s", ch.Type)
	}

	// Record digest in outbox for audit trail
	if d.outbox != nil {
		ids := make([]string, len(payloads))
		for i, p := range payloads {
			ids[i] = p.IncidentID
		}
		digestJSON, _ := json.Marshal(payloads)
		evt := monitoring.NotificationEvent{
			IncidentID:  strings.Join(ids, ","),
			Channel:     fmt.Sprintf("%s:%s", ch.Type, ch.Name),
			PayloadJSON: string(digestJSON),
		}
		if err != nil {
			evt.LastError = err.Error()
		}
		if enqErr := d.outbox.Enqueue(evt); enqErr != nil {
			d.logger.Printf("notification: failed to record digest in outbox: %v", enqErr)
		}
	}

	if err != nil {
		d.logger.Printf("notification: failed to send digest (%d incidents) to %s channel %q: %v", len(payloads), ch.Type, ch.Name, err)
	} else {
		d.logger.Printf("notification: sent digest (%d incidents) to %s channel %q", len(payloads), ch.Type, ch.Name)
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

	statusIndicator := "[CRITICAL]"
	if p.Status == "resolved" {
		statusIndicator = "[RESOLVED]"
	} else if p.Severity == "warning" {
		statusIndicator = "[WARNING]"
	}

	title := fmt.Sprintf("%s %s — %s", statusIndicator, strings.ToUpper(p.Status), p.CheckName)

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

	htmlBody := buildHTMLEmail(p, d.dashboardURL)

	from := ch.FromEmail
	if from == "" {
		from = ch.SMTPUser
	}

	recipients := strings.Split(ch.Email, ",")
	for i := range recipients {
		recipients[i] = strings.TrimSpace(recipients[i])
	}

	boundary := fmt.Sprintf("healthops-%d", time.Now().UnixNano())
	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n--%s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n\r\n--%s\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n\r\n--%s--",
		from, strings.Join(recipients, ","), subject,
		boundary,
		boundary, buildPlainTextFallback(p),
		boundary, htmlBody,
		boundary,
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
	statusIndicator := "[CRITICAL]"
	if p.Status == "resolved" {
		statusIndicator = "[RESOLVED]"
	} else if p.Severity == "warning" {
		statusIndicator = "[WARNING]"
	}

	text := fmt.Sprintf(
		"%s *%s — %s*\n\n*Severity:* %s\n*Type:* %s\n*Server:* %s\n\n%s",
		statusIndicator, strings.ToUpper(p.Status), escapeMarkdown(p.CheckName),
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

func buildPayload(incident monitoring.Incident, status string) NotificationPayload {
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

func buildPlainTextFallback(p NotificationPayload) string {
	lines := []string{
		fmt.Sprintf("Incident: %s", p.IncidentID),
		fmt.Sprintf("Check: %s (%s)", p.CheckName, p.CheckType),
		fmt.Sprintf("Severity: %s", strings.ToUpper(p.Severity)),
	}
	if p.Server != "" {
		lines = append(lines, fmt.Sprintf("Server: %s", p.Server))
	}
	lines = append(lines,
		fmt.Sprintf("Status: %s", strings.ToUpper(p.Status)),
		fmt.Sprintf("Started: %s", p.StartedAt),
		"",
		p.Message,
	)
	if p.ResolvedAt != "" {
		lines = append(lines, fmt.Sprintf("\nResolved: %s", p.ResolvedAt))
	}
	return strings.Join(lines, "\n")
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

// --- Digest helpers ---

// digestSummary returns severity counts and the highest severity in the batch.
func digestSummary(payloads []NotificationPayload) (critical, warning, other int, highest string) {
	for _, p := range payloads {
		switch p.Severity {
		case "critical":
			critical++
		case "warning":
			warning++
		default:
			other++
		}
	}
	if critical > 0 {
		highest = "critical"
	} else if warning > 0 {
		highest = "warning"
	} else {
		highest = "info"
	}
	return
}

// --- Slack Digest ---

func (d *NotificationDispatcher) sendSlackDigest(ch NotificationChannelConfig, payloads []NotificationPayload) error {
	critical, warning, _, highest := digestSummary(payloads)

	color := "#e01e5a"
	if highest == "warning" {
		color = "#ecb22e"
	}

	title := fmt.Sprintf("[ALERT] %d checks failing", len(payloads))
	summary := ""
	if critical > 0 {
		summary += fmt.Sprintf("%d critical", critical)
	}
	if warning > 0 {
		if summary != "" {
			summary += ", "
		}
		summary += fmt.Sprintf("%d warning", warning)
	}

	var lines []string
	for _, p := range payloads {
		sev := strings.ToUpper(p.Severity)
		line := fmt.Sprintf("- *%s* [%s] %s", p.CheckName, sev, p.Message)
		if p.Server != "" {
			line += fmt.Sprintf(" (server: %s)", p.Server)
		}
		lines = append(lines, line)
	}

	slackBody := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color":  color,
				"title":  title,
				"text":   strings.Join(lines, "\n"),
				"footer": fmt.Sprintf("HealthOps | %s", summary),
				"ts":     time.Now().Unix(),
			},
		},
	}

	return d.postJSON(ch.WebhookURL, slackBody, nil)
}

// --- Discord Digest ---

func (d *NotificationDispatcher) sendDiscordDigest(ch NotificationChannelConfig, payloads []NotificationPayload) error {
	_, _, _, highest := digestSummary(payloads)

	color := 0xe01e5a
	if highest == "warning" {
		color = 0xecb22e
	}

	var lines []string
	for _, p := range payloads {
		sev := strings.ToUpper(p.Severity)
		line := fmt.Sprintf("**%s** [%s] %s", p.CheckName, sev, p.Message)
		if p.Server != "" {
			line += fmt.Sprintf(" (server: %s)", p.Server)
		}
		lines = append(lines, line)
	}

	discordBody := map[string]interface{}{
		"embeds": []map[string]interface{}{
			{
				"title":       fmt.Sprintf("ALERT — %d checks failing", len(payloads)),
				"description": strings.Join(lines, "\n"),
				"color":       color,
				"footer":      map[string]string{"text": "HealthOps"},
				"timestamp":   time.Now().Format(time.RFC3339),
			},
		},
	}

	return d.postJSON(ch.WebhookURL, discordBody, nil)
}

// --- Webhook Digest ---

func (d *NotificationDispatcher) sendWebhookDigest(ch NotificationChannelConfig, payloads []NotificationPayload) error {
	body := map[string]interface{}{
		"type":      "digest",
		"count":     len(payloads),
		"incidents": payloads,
		"timestamp": time.Now().Format(time.RFC3339),
	}
	return d.postJSON(ch.WebhookURL, body, ch.Headers)
}

// --- Telegram Digest ---

func (d *NotificationDispatcher) sendTelegramDigest(ch NotificationChannelConfig, payloads []NotificationPayload) error {
	critical, warning, _, _ := digestSummary(payloads)

	header := fmt.Sprintf("[ALERT] *%d checks failing*", len(payloads))
	var counts []string
	if critical > 0 {
		counts = append(counts, fmt.Sprintf("%d critical", critical))
	}
	if warning > 0 {
		counts = append(counts, fmt.Sprintf("%d warning", warning))
	}
	if len(counts) > 0 {
		header += fmt.Sprintf(" \\(%s\\)", strings.Join(counts, ", "))
	}

	var lines []string
	for _, p := range payloads {
		sev := strings.ToUpper(p.Severity)
		line := fmt.Sprintf("\\- *%s* \\[%s\\] %s", escapeMarkdown(p.CheckName), sev, escapeMarkdown(p.Message))
		lines = append(lines, line)
	}

	text := header + "\n\n" + strings.Join(lines, "\n")

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", ch.BotToken)
	body := map[string]interface{}{
		"chat_id":    ch.ChatID,
		"text":       text,
		"parse_mode": "MarkdownV2",
	}

	return d.postJSON(url, body, nil)
}

// --- Email Digest ---

func (d *NotificationDispatcher) sendEmailDigest(ch NotificationChannelConfig, payloads []NotificationPayload) error {
	critical, warning, _, highest := digestSummary(payloads)

	subject := fmt.Sprintf("[HealthOps] ALERT — %d checks failing", len(payloads))
	if critical > 0 {
		subject += fmt.Sprintf(" (%d critical)", critical)
	} else if warning > 0 {
		subject += fmt.Sprintf(" (%d warning)", warning)
	}

	htmlBody := buildDigestHTMLEmail(payloads, critical, warning, highest, d.dashboardURL)
	plainBody := buildDigestPlainText(payloads)

	from := ch.FromEmail
	if from == "" {
		from = ch.SMTPUser
	}

	recipients := strings.Split(ch.Email, ",")
	for i := range recipients {
		recipients[i] = strings.TrimSpace(recipients[i])
	}

	boundary := fmt.Sprintf("healthops-%d", time.Now().UnixNano())
	msg := fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=\"%s\"\r\n\r\n--%s\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s\r\n\r\n--%s\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s\r\n\r\n--%s--",
		from, strings.Join(recipients, ","), subject,
		boundary,
		boundary, plainBody,
		boundary, htmlBody,
		boundary,
	)

	addr := fmt.Sprintf("%s:%d", ch.SMTPHost, ch.SMTPPort)
	var auth smtp.Auth
	if ch.SMTPUser != "" {
		auth = smtp.PlainAuth("", ch.SMTPUser, ch.SMTPPass, ch.SMTPHost)
	}

	return smtp.SendMail(addr, auth, from, recipients, []byte(msg))
}

func buildDigestPlainText(payloads []NotificationPayload) string {
	lines := []string{
		fmt.Sprintf("HealthOps Alert — %d checks failing", len(payloads)),
		strings.Repeat("=", 50),
		"",
	}
	for i, p := range payloads {
		lines = append(lines,
			fmt.Sprintf("%d. %s [%s]", i+1, p.CheckName, strings.ToUpper(p.Severity)),
			fmt.Sprintf("   %s", p.Message),
		)
		if p.Server != "" {
			lines = append(lines, fmt.Sprintf("   Server: %s", p.Server))
		}
		lines = append(lines, fmt.Sprintf("   Started: %s", p.StartedAt), "")
	}
	return strings.Join(lines, "\n")
}
