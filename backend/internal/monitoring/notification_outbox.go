package monitoring

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// NotificationOutboxRepository defines generic notification queue operations.
// This is reusable across all check types, not MySQL-specific.
type NotificationOutboxRepository interface {
	Enqueue(evt NotificationEvent) error
	ListPending(limit int) ([]NotificationEvent, error)
	MarkSent(id string) error
	MarkFailed(id string, reason string) error
	PruneBefore(cutoff time.Time) error
}

// FileNotificationOutbox implements NotificationOutboxRepository with JSONL backing.
type FileNotificationOutbox struct {
	mu   sync.RWMutex
	path string
	data []NotificationEvent
}

// NewFileNotificationOutbox creates a file-backed notification outbox.
func NewFileNotificationOutbox(path string) (*FileNotificationOutbox, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create outbox dir: %w", err)
	}

	outbox := &FileNotificationOutbox{path: path}
	var err error
	outbox.data, err = loadJSONLFile[NotificationEvent](path)
	if err != nil {
		return nil, fmt.Errorf("load outbox: %w", err)
	}
	return outbox, nil
}

func (o *FileNotificationOutbox) Enqueue(evt NotificationEvent) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if evt.NotificationID == "" {
		evt.NotificationID = fmt.Sprintf("notif-%s-%d", evt.IncidentID, time.Now().UnixNano())
	}
	if evt.Status == "" {
		evt.Status = "pending"
	}
	if evt.CreatedAt.IsZero() {
		evt.CreatedAt = time.Now().UTC()
	}

	o.data = append(o.data, evt)
	return appendJSONLFile(o.path, evt)
}

func (o *FileNotificationOutbox) ListPending(limit int) ([]NotificationEvent, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	var result []NotificationEvent
	for _, evt := range o.data {
		if evt.Status == "pending" {
			result = append(result, evt)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (o *FileNotificationOutbox) MarkSent(id string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	found := false
	for i := range o.data {
		if o.data[i].NotificationID == id {
			if o.data[i].Status != "pending" {
				return fmt.Errorf("notification %s is not pending (status: %s)", id, o.data[i].Status)
			}
			o.data[i].Status = "sent"
			now := time.Now().UTC()
			o.data[i].SentAt = &now
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("notification not found: %s", id)
	}

	return rewriteJSONLFile(o.path, o.data)
}

func (o *FileNotificationOutbox) MarkFailed(id string, reason string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	found := false
	for i := range o.data {
		if o.data[i].NotificationID == id {
			o.data[i].Status = "failed"
			o.data[i].LastError = reason
			o.data[i].RetryCount++
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("notification not found: %s", id)
	}

	return rewriteJSONLFile(o.path, o.data)
}

func (o *FileNotificationOutbox) PruneBefore(cutoff time.Time) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	pruned := o.data[:0]
	for _, evt := range o.data {
		if !evt.CreatedAt.Before(cutoff) {
			pruned = append(pruned, evt)
		}
	}
	o.data = pruned
	return rewriteJSONLFile(o.path, o.data)
}

// EnqueueIncidentNotification is a helper that creates a notification event from an incident.
// Generic — works for any incident type, not just MySQL.
func EnqueueIncidentNotification(outbox NotificationOutboxRepository, incident Incident, channel string) error {
	payload := fmt.Sprintf(`{"incidentId":%q,"checkId":%q,"checkName":%q,"severity":%q,"message":%q,"status":%q}`,
		incident.ID, incident.CheckID, incident.CheckName, incident.Severity, incident.Message, incident.Status)

	return outbox.Enqueue(NotificationEvent{
		IncidentID:  incident.ID,
		Channel:     channel,
		PayloadJSON: payload,
	})
}
