package notify

import (
	"testing"
	"time"

	"medics-health-check/backend/internal/monitoring"
)

type memoryNotificationOutbox struct {
	events []monitoring.NotificationEvent
}

func (o *memoryNotificationOutbox) Enqueue(evt monitoring.NotificationEvent) error {
	if evt.NotificationID == "" {
		evt.NotificationID = "notif-test"
	}
	if evt.Status == "" {
		evt.Status = "pending"
	}
	if evt.CreatedAt.IsZero() {
		evt.CreatedAt = time.Now().UTC()
	}
	o.events = append(o.events, evt)
	return nil
}

func (o *memoryNotificationOutbox) ListPending(limit int) ([]monitoring.NotificationEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	var out []monitoring.NotificationEvent
	for _, evt := range o.events {
		if evt.Status == "pending" {
			out = append(out, evt)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (o *memoryNotificationOutbox) MarkSent(id string) error {
	return nil
}

func (o *memoryNotificationOutbox) MarkFailed(id string, reason string) error {
	return nil
}

func (o *memoryNotificationOutbox) PruneBefore(cutoff time.Time) error {
	return nil
}

func TestEnqueueIncidentNotification(t *testing.T) {
	outbox := &memoryNotificationOutbox{}
	incident := monitoring.Incident{
		ID:        "inc-1",
		CheckID:   "check-1",
		CheckName: "API",
		Severity:  "critical",
		Message:   "down",
		Status:    "open",
	}

	if err := EnqueueIncidentNotification(outbox, incident, "slack"); err != nil {
		t.Fatalf("EnqueueIncidentNotification() error = %v", err)
	}
	if len(outbox.events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(outbox.events))
	}
	if outbox.events[0].IncidentID != incident.ID {
		t.Fatalf("IncidentID = %q, want %q", outbox.events[0].IncidentID, incident.ID)
	}
	if outbox.events[0].Channel != "slack" {
		t.Fatalf("Channel = %q, want slack", outbox.events[0].Channel)
	}
}
