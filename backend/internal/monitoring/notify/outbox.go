package notify

import (
	"fmt"
	"time"

	"medics-health-check/backend/internal/monitoring"
)

// NotificationOutboxRepository defines generic notification queue operations.
// This is reusable across all check types, not MySQL-specific.
type NotificationOutboxRepository interface {
	Enqueue(evt monitoring.NotificationEvent) error
	ListPending(limit int) ([]monitoring.NotificationEvent, error)
	MarkSent(id string) error
	MarkFailed(id string, reason string) error
	PruneBefore(cutoff time.Time) error
}

// EnqueueIncidentNotification creates a notification event from an incident.
func EnqueueIncidentNotification(outbox NotificationOutboxRepository, incident monitoring.Incident, channel string) error {
	payload := fmt.Sprintf(`{"incidentId":%q,"checkId":%q,"checkName":%q,"severity":%q,"message":%q,"status":%q}`,
		incident.ID, incident.CheckID, incident.CheckName, incident.Severity, incident.Message, incident.Status)

	return outbox.Enqueue(monitoring.NotificationEvent{
		IncidentID:  incident.ID,
		Channel:     channel,
		PayloadJSON: payload,
	})
}
