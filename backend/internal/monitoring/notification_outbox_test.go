package monitoring

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileNotificationOutbox_EnqueueAndListPending(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outbox.jsonl")

	outbox, err := NewFileNotificationOutbox(path)
	if err != nil {
		t.Fatalf("create outbox: %v", err)
	}

	evt := NotificationEvent{
		IncidentID:  "inc-1",
		Channel:     "slack",
		PayloadJSON: `{"msg":"test"}`,
	}

	if err := outbox.Enqueue(evt); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	pending, err := outbox.ListPending(10)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if pending[0].Status != "pending" {
		t.Errorf("expected status pending, got %s", pending[0].Status)
	}
	if pending[0].Channel != "slack" {
		t.Errorf("expected channel slack, got %s", pending[0].Channel)
	}
}

func TestFileNotificationOutbox_AutoGeneratesID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outbox.jsonl")

	outbox, _ := NewFileNotificationOutbox(path)
	_ = outbox.Enqueue(NotificationEvent{IncidentID: "inc-1", Channel: "email"})

	pending, _ := outbox.ListPending(10)
	if len(pending) != 1 {
		t.Fatal("expected 1 pending")
	}
	if pending[0].NotificationID == "" {
		t.Error("expected auto-generated notification ID")
	}
}

func TestFileNotificationOutbox_MarkSent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outbox.jsonl")

	outbox, _ := NewFileNotificationOutbox(path)
	_ = outbox.Enqueue(NotificationEvent{
		NotificationID: "notif-1",
		IncidentID:     "inc-1",
		Channel:        "slack",
	})

	if err := outbox.MarkSent("notif-1"); err != nil {
		t.Fatalf("mark sent: %v", err)
	}

	// Should no longer be pending
	pending, _ := outbox.ListPending(10)
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after mark sent, got %d", len(pending))
	}
}

func TestFileNotificationOutbox_MarkSentTwice(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outbox.jsonl")

	outbox, _ := NewFileNotificationOutbox(path)
	_ = outbox.Enqueue(NotificationEvent{
		NotificationID: "notif-1",
		IncidentID:     "inc-1",
		Channel:        "slack",
	})

	_ = outbox.MarkSent("notif-1")
	err := outbox.MarkSent("notif-1")
	if err == nil {
		t.Fatal("expected error marking already-sent notification")
	}
}

func TestFileNotificationOutbox_MarkSentNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outbox.jsonl")

	outbox, _ := NewFileNotificationOutbox(path)
	err := outbox.MarkSent("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent notification")
	}
}

func TestFileNotificationOutbox_MarkFailed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outbox.jsonl")

	outbox, _ := NewFileNotificationOutbox(path)
	_ = outbox.Enqueue(NotificationEvent{
		NotificationID: "notif-1",
		IncidentID:     "inc-1",
		Channel:        "slack",
	})

	if err := outbox.MarkFailed("notif-1", "timeout"); err != nil {
		t.Fatalf("mark failed: %v", err)
	}

	// Should no longer be pending
	pending, _ := outbox.ListPending(10)
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after mark failed, got %d", len(pending))
	}
}

func TestFileNotificationOutbox_MarkFailedNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outbox.jsonl")

	outbox, _ := NewFileNotificationOutbox(path)
	err := outbox.MarkFailed("nonexistent", "reason")
	if err == nil {
		t.Fatal("expected error for nonexistent notification")
	}
}

func TestFileNotificationOutbox_PruneBefore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outbox.jsonl")

	outbox, _ := NewFileNotificationOutbox(path)

	now := time.Now().UTC()
	_ = outbox.Enqueue(NotificationEvent{
		NotificationID: "old",
		IncidentID:     "inc-old",
		Channel:        "slack",
		CreatedAt:      now.Add(-48 * time.Hour),
	})
	_ = outbox.Enqueue(NotificationEvent{
		NotificationID: "new",
		IncidentID:     "inc-new",
		Channel:        "slack",
		CreatedAt:      now,
	})

	cutoff := now.Add(-24 * time.Hour)
	if err := outbox.PruneBefore(cutoff); err != nil {
		t.Fatalf("prune: %v", err)
	}

	pending, _ := outbox.ListPending(10)
	if len(pending) != 1 {
		t.Errorf("expected 1 pending after prune, got %d", len(pending))
	}
}

func TestFileNotificationOutbox_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outbox.jsonl")

	outbox1, _ := NewFileNotificationOutbox(path)
	_ = outbox1.Enqueue(NotificationEvent{
		NotificationID: "persist-test",
		IncidentID:     "inc-1",
		Channel:        "webhook",
	})

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("JSONL file not created")
	}

	outbox2, err := NewFileNotificationOutbox(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	pending, _ := outbox2.ListPending(10)
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending after reload, got %d", len(pending))
	}
	if pending[0].NotificationID != "persist-test" {
		t.Errorf("expected persist-test, got %s", pending[0].NotificationID)
	}
}

func TestFileNotificationOutbox_ListPendingLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outbox.jsonl")

	outbox, _ := NewFileNotificationOutbox(path)
	for i := 0; i < 10; i++ {
		_ = outbox.Enqueue(NotificationEvent{IncidentID: "inc", Channel: "slack"})
	}

	pending, _ := outbox.ListPending(3)
	if len(pending) != 3 {
		t.Errorf("expected 3 with limit, got %d", len(pending))
	}
}

func TestEnqueueIncidentNotification(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outbox.jsonl")

	outbox, _ := NewFileNotificationOutbox(path)
	incident := Incident{
		ID:       "inc-1",
		CheckID:  "mysql-1",
		Severity: "critical",
		Message:  "Connection utilization high",
		Status:   "open",
	}

	if err := EnqueueIncidentNotification(outbox, incident, "slack"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	pending, _ := outbox.ListPending(10)
	if len(pending) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(pending))
	}
	if pending[0].Channel != "slack" {
		t.Errorf("expected channel slack, got %s", pending[0].Channel)
	}
	if pending[0].PayloadJSON == "" {
		t.Error("expected non-empty payload")
	}
}
