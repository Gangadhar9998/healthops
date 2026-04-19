package monitoring

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func newTestMySQLAPIHandler(t *testing.T) (*MySQLAPIHandler, *FileMySQLRepository) {
	t.Helper()
	dir := t.TempDir()

	mysqlRepo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create mysql repo: %v", err)
	}

	snapshotRepo, err := NewFileSnapshotRepository(filepath.Join(dir, "snapshots.jsonl"))
	if err != nil {
		t.Fatalf("create snapshot repo: %v", err)
	}

	outbox, err := NewFileNotificationOutbox(filepath.Join(dir, "outbox.jsonl"))
	if err != nil {
		t.Fatalf("create outbox: %v", err)
	}

	aiQueue, err := NewFileAIQueue(dir)
	if err != nil {
		t.Fatalf("create ai queue: %v", err)
	}

	cfg := &Config{
		Auth: AuthConfig{Enabled: true, Username: "admin", Password: "secret"},
	}

	handler := NewMySQLAPIHandler(mysqlRepo, snapshotRepo, outbox, aiQueue, nil, cfg)
	return handler, mysqlRepo
}

func TestMySQLSamplesEndpoint_GET(t *testing.T) {
	handler, repo := newTestMySQLAPIHandler(t)

	now := time.Now().UTC()
	_, _ = repo.AppendSample(MySQLSample{
		SampleID: "s1", CheckID: "mysql-1", Timestamp: now,
		Connections: 50, MaxConnections: 100,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mysql/samples?checkId=mysql-1", nil)
	rec := httptest.NewRecorder()
	handler.handleMySQLSamples(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected success")
	}
}

func TestMySQLSamplesEndpoint_MissingCheckID(t *testing.T) {
	handler, _ := newTestMySQLAPIHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mysql/samples", nil)
	rec := httptest.NewRecorder()
	handler.handleMySQLSamples(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestMySQLSamplesEndpoint_MethodNotAllowed(t *testing.T) {
	handler, _ := newTestMySQLAPIHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mysql/samples?checkId=x", nil)
	rec := httptest.NewRecorder()
	handler.handleMySQLSamples(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestMySQLDeltasEndpoint_GET(t *testing.T) {
	handler, repo := newTestMySQLAPIHandler(t)

	now := time.Now().UTC()
	_, _ = repo.AppendSample(MySQLSample{
		SampleID: "s1", CheckID: "mysql-1", Timestamp: now.Add(-15 * time.Second),
		Questions: 1000,
	})
	_, _ = repo.AppendSample(MySQLSample{
		SampleID: "s2", CheckID: "mysql-1", Timestamp: now,
		Questions: 1150,
	})
	_, _ = repo.ComputeAndAppendDelta("s2")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mysql/deltas?checkId=mysql-1", nil)
	rec := httptest.NewRecorder()
	handler.handleMySQLDeltas(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp APIResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.Success {
		t.Fatal("expected success")
	}
}

func TestMySQLDeltasEndpoint_MissingCheckID(t *testing.T) {
	handler, _ := newTestMySQLAPIHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mysql/deltas", nil)
	rec := httptest.NewRecorder()
	handler.handleMySQLDeltas(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestIncidentSnapshotsEndpoint_GET(t *testing.T) {
	handler, _ := newTestMySQLAPIHandler(t)

	// Save some snapshots via the underlying repo
	snapshotRepo := handler.snapshotRepo.(*FileSnapshotRepository)
	_ = snapshotRepo.SaveSnapshots("inc-1", []IncidentSnapshot{
		{SnapshotType: "latest_sample", Timestamp: time.Now().UTC(), PayloadJSON: `{"test":1}`},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/inc-1/snapshots", nil)
	rec := httptest.NewRecorder()
	handler.handleIncidentSnapshots(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestNotificationsEndpoint_GET(t *testing.T) {
	handler, _ := newTestMySQLAPIHandler(t)

	// Enqueue a notification
	outbox := handler.outbox.(*FileNotificationOutbox)
	_ = outbox.Enqueue(NotificationEvent{
		NotificationID: "notif-1",
		IncidentID:     "inc-1",
		Channel:        "slack",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
	rec := httptest.NewRecorder()
	handler.handleNotifications(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp APIResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if !resp.Success {
		t.Fatal("expected success")
	}
}

func TestNotificationSentEndpoint_RequiresAuth(t *testing.T) {
	handler, _ := newTestMySQLAPIHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/notif-1/sent", nil)
	rec := httptest.NewRecorder()
	handler.handleNotificationByID(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", rec.Code)
	}
}

func TestNotificationSentEndpoint_WithAuth(t *testing.T) {
	handler, _ := newTestMySQLAPIHandler(t)

	// Enqueue first
	outbox := handler.outbox.(*FileNotificationOutbox)
	_ = outbox.Enqueue(NotificationEvent{
		NotificationID: "notif-1",
		IncidentID:     "inc-1",
		Channel:        "slack",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/notif-1/sent", nil)
	req.SetBasicAuth("admin", "secret")
	rec := httptest.NewRecorder()
	handler.handleNotificationByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestNotificationFailedEndpoint_WithAuth(t *testing.T) {
	handler, _ := newTestMySQLAPIHandler(t)

	outbox := handler.outbox.(*FileNotificationOutbox)
	_ = outbox.Enqueue(NotificationEvent{
		NotificationID: "notif-1",
		IncidentID:     "inc-1",
		Channel:        "slack",
	})

	body := bytes.NewBufferString(`{"reason":"timeout"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/notif-1/failed", body)
	req.SetBasicAuth("admin", "secret")
	rec := httptest.NewRecorder()
	handler.handleNotificationByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestNotificationEndpoint_MethodNotAllowed(t *testing.T) {
	handler, _ := newTestMySQLAPIHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/notif-1/sent", nil)
	rec := httptest.NewRecorder()
	handler.handleNotificationByID(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestNotificationEndpoint_UnknownAction(t *testing.T) {
	handler, _ := newTestMySQLAPIHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/notif-1/unknown", nil)
	req.SetBasicAuth("admin", "secret")
	rec := httptest.NewRecorder()
	handler.handleNotificationByID(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestAIQueueEndpoint_GET(t *testing.T) {
	handler, _ := newTestMySQLAPIHandler(t)

	_ = handler.aiQueue.Enqueue("inc-1", "v1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai/queue", nil)
	rec := httptest.NewRecorder()
	handler.handleAIQueue(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestAIQueueDoneEndpoint_RequiresAuth(t *testing.T) {
	handler, _ := newTestMySQLAPIHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/queue/inc-1/done", nil)
	rec := httptest.NewRecorder()
	handler.handleAIQueueByID(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without auth, got %d", rec.Code)
	}
}

func TestAIQueueDoneEndpoint_WithAuth(t *testing.T) {
	handler, _ := newTestMySQLAPIHandler(t)

	_ = handler.aiQueue.Enqueue("inc-1", "v1")
	_, _ = handler.aiQueue.ClaimPending(1)

	body := bytes.NewBufferString(`{"analysis":"root cause found","suggestions":["fix it"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/queue/inc-1/done", body)
	req.SetBasicAuth("admin", "secret")
	rec := httptest.NewRecorder()
	handler.handleAIQueueByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAIQueueFailedEndpoint_WithAuth(t *testing.T) {
	handler, _ := newTestMySQLAPIHandler(t)

	_ = handler.aiQueue.Enqueue("inc-1", "v1")
	_, _ = handler.aiQueue.ClaimPending(1)

	body := bytes.NewBufferString(`{"reason":"API unavailable"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/queue/inc-1/failed", body)
	req.SetBasicAuth("admin", "secret")
	rec := httptest.NewRecorder()
	handler.handleAIQueueByID(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAIQueueEndpoint_MethodNotAllowed(t *testing.T) {
	handler, _ := newTestMySQLAPIHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/queue", nil)
	rec := httptest.NewRecorder()
	handler.handleAIQueue(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rec.Code)
	}
}

func TestAIQueueEndpoint_UnknownAction(t *testing.T) {
	handler, _ := newTestMySQLAPIHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/queue/inc-1/unknown", nil)
	req.SetBasicAuth("admin", "secret")
	rec := httptest.NewRecorder()
	handler.handleAIQueueByID(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// Test all new endpoints follow the APIResponse envelope contract
func TestMySQLEndpointsFollowEnvelopeContract(t *testing.T) {
	handler, repo := newTestMySQLAPIHandler(t)

	now := time.Now().UTC()
	_, _ = repo.AppendSample(MySQLSample{
		SampleID: "s1", CheckID: "mysql-1", Timestamp: now,
	})

	endpoints := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
	}{
		{"samples", http.MethodGet, "/api/v1/mysql/samples?checkId=mysql-1", handler.handleMySQLSamples},
		{"deltas", http.MethodGet, "/api/v1/mysql/deltas?checkId=mysql-1", handler.handleMySQLDeltas},
		{"notifications", http.MethodGet, "/api/v1/notifications", handler.handleNotifications},
		{"ai queue", http.MethodGet, "/api/v1/ai/queue", handler.handleAIQueue},
	}

	for _, ep := range endpoints {
		t.Run(ep.name, func(t *testing.T) {
			req := httptest.NewRequest(ep.method, ep.path, nil)
			rec := httptest.NewRecorder()
			ep.handler(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}

			var resp APIResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode envelope: %v", err)
			}
			if !resp.Success {
				t.Error("expected success=true in envelope")
			}
		})
	}
}

// Test that all mutating endpoints require auth
func TestMySQLMutatingEndpointsRequireAuth(t *testing.T) {
	handler, _ := newTestMySQLAPIHandler(t)

	mutatingEndpoints := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
		body    string
	}{
		{"notification sent", http.MethodPost, "/api/v1/notifications/notif-1/sent", handler.handleNotificationByID, ""},
		{"notification failed", http.MethodPost, "/api/v1/notifications/notif-1/failed", handler.handleNotificationByID, `{"reason":"test"}`},
		{"ai queue done", http.MethodPost, "/api/v1/ai/queue/inc-1/done", handler.handleAIQueueByID, `{"analysis":"test"}`},
		{"ai queue failed", http.MethodPost, "/api/v1/ai/queue/inc-1/failed", handler.handleAIQueueByID, `{"reason":"test"}`},
	}

	for _, ep := range mutatingEndpoints {
		t.Run(ep.name+"/no auth", func(t *testing.T) {
			var body *bytes.Buffer
			if ep.body != "" {
				body = bytes.NewBufferString(ep.body)
			} else {
				body = &bytes.Buffer{}
			}

			req := httptest.NewRequest(ep.method, ep.path, body)
			rec := httptest.NewRecorder()
			ep.handler(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 without auth, got %d", rec.Code)
			}
		})
	}
}
