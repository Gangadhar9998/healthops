package monitoring

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Part 1: Contract Tests (spec Gate G3)
// ---------------------------------------------------------------------------

func TestContractMySQLSamplesEndpoint(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		_, err := repo.AppendSample(MySQLSample{
			SampleID:       "s" + string(rune('1'+i)),
			CheckID:        "test-db",
			Timestamp:      now.Add(time.Duration(i) * time.Minute),
			Connections:    int64(100 + i),
			MaxConnections: 200,
		})
		if err != nil {
			t.Fatalf("append sample %d: %v", i, err)
		}
	}

	cfg := &Config{}
	h := NewMySQLAPIHandler(repo, nil, nil, nil, nil, cfg)

	t.Run("success with limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/mysql/samples?checkId=test-db&limit=2", nil)
		rec := httptest.NewRecorder()
		h.handleMySQLSamples(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var samples []MySQLSample
		if err := decodeAPIResponseData(rec.Body.Bytes(), &samples); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(samples) != 2 {
			t.Fatalf("expected 2 samples, got %d", len(samples))
		}
	})

	t.Run("missing checkId returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/mysql/samples", nil)
		rec := httptest.NewRecorder()
		h.handleMySQLSamples(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}

		resp, err := decodeAPIResponse(rec.Body.Bytes())
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Success {
			t.Fatal("expected success=false")
		}
	})

	t.Run("POST returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/mysql/samples?checkId=test-db", nil)
		rec := httptest.NewRecorder()
		h.handleMySQLSamples(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", rec.Code)
		}
	})
}

func TestContractMySQLDeltasEndpoint(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	now := time.Now().UTC()
	s1 := MySQLSample{
		SampleID:    "s1",
		CheckID:     "test-db",
		Timestamp:   now,
		Connections: 100, MaxConnections: 200,
		SlowQueries: 10, Questions: 1000,
	}
	s2 := MySQLSample{
		SampleID:    "s2",
		CheckID:     "test-db",
		Timestamp:   now.Add(time.Minute),
		Connections: 110, MaxConnections: 200,
		SlowQueries: 15, Questions: 2000,
	}

	id1, _ := repo.AppendSample(s1)
	id2, _ := repo.AppendSample(s2)
	_ = id1
	if _, err := repo.ComputeAndAppendDelta(id2); err != nil {
		t.Fatalf("compute delta: %v", err)
	}

	cfg := &Config{}
	h := NewMySQLAPIHandler(repo, nil, nil, nil, nil, cfg)

	t.Run("success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/mysql/deltas?checkId=test-db", nil)
		rec := httptest.NewRecorder()
		h.handleMySQLDeltas(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var deltas []MySQLDelta
		if err := decodeAPIResponseData(rec.Body.Bytes(), &deltas); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(deltas) != 1 {
			t.Fatalf("expected 1 delta, got %d", len(deltas))
		}
	})

	t.Run("missing checkId returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/mysql/deltas", nil)
		rec := httptest.NewRecorder()
		h.handleMySQLDeltas(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}

func TestContractIncidentSnapshotsEndpoint(t *testing.T) {
	dir := t.TempDir()
	snapPath := filepath.Join(dir, "snapshots.jsonl")
	snapRepo, err := NewFileSnapshotRepository(snapPath)
	if err != nil {
		t.Fatalf("create snapshot repo: %v", err)
	}

	now := time.Now().UTC()
	snaps := []IncidentSnapshot{
		{IncidentID: "inc-1", SnapshotType: "sample", Timestamp: now, PayloadJSON: `{"a":1}`},
		{IncidentID: "inc-1", SnapshotType: "delta", Timestamp: now.Add(time.Second), PayloadJSON: `{"b":2}`},
		{IncidentID: "inc-1", SnapshotType: "processlist", Timestamp: now.Add(2 * time.Second), PayloadJSON: `{"c":3}`},
	}
	if err := snapRepo.SaveSnapshots("inc-1", snaps); err != nil {
		t.Fatalf("save snapshots: %v", err)
	}

	cfg := &Config{}
	h := NewMySQLAPIHandler(nil, snapRepo, nil, nil, nil, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents/inc-1/snapshots", nil)
	rec := httptest.NewRecorder()
	h.handleIncidentSnapshots(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var result []IncidentSnapshot
	if err := decodeAPIResponseData(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(result))
	}
}

func TestContractNotificationsEndpoint(t *testing.T) {
	dir := t.TempDir()
	outboxPath := filepath.Join(dir, "notifications.jsonl")
	outbox, err := NewFileNotificationOutbox(outboxPath)
	if err != nil {
		t.Fatalf("create outbox: %v", err)
	}

	for i := 0; i < 2; i++ {
		err := outbox.Enqueue(NotificationEvent{
			NotificationID: "notif-" + string(rune('1'+i)),
			IncidentID:     "inc-1",
			Channel:        "webhook",
			PayloadJSON:    `{"msg":"alert"}`,
			Status:         "pending",
			CreatedAt:      time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}

	cfg := &Config{}
	h := NewMySQLAPIHandler(nil, nil, outbox, nil, nil, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?limit=10", nil)
	rec := httptest.NewRecorder()
	h.handleNotifications(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var events []NotificationEvent
	if err := decodeAPIResponseData(rec.Body.Bytes(), &events); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(events))
	}
}

func TestContractAIQueueEndpoint(t *testing.T) {
	dir := t.TempDir()
	aiQueue, err := NewFileAIQueue(dir)
	if err != nil {
		t.Fatalf("create ai queue: %v", err)
	}

	if err := aiQueue.Enqueue("inc-1", "v1"); err != nil {
		t.Fatalf("enqueue 1: %v", err)
	}
	if err := aiQueue.Enqueue("inc-2", "v1"); err != nil {
		t.Fatalf("enqueue 2: %v", err)
	}

	cfg := &Config{}
	h := NewMySQLAPIHandler(nil, nil, nil, aiQueue, nil, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai/queue?limit=10", nil)
	rec := httptest.NewRecorder()
	h.handleAIQueue(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var items []AIQueueItem
	if err := decodeAPIResponseData(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestContractMutatingEndpointsRequireAuth(t *testing.T) {
	dir := t.TempDir()
	outboxPath := filepath.Join(dir, "notifications.jsonl")
	outbox, err := NewFileNotificationOutbox(outboxPath)
	if err != nil {
		t.Fatalf("create outbox: %v", err)
	}

	// Enqueue a notification so mark-sent/failed have a target
	_ = outbox.Enqueue(NotificationEvent{
		NotificationID: "notif-1",
		IncidentID:     "inc-1",
		Channel:        "webhook",
		PayloadJSON:    `{"msg":"test"}`,
	})

	aiQueue, err := NewFileAIQueue(dir)
	if err != nil {
		t.Fatalf("create ai queue: %v", err)
	}
	_ = aiQueue.Enqueue("inc-1", "v1")

	cfg := &Config{
		Auth: AuthConfig{Enabled: true, Username: "admin", Password: "pass"},
	}
	h := NewMySQLAPIHandler(nil, nil, outbox, aiQueue, nil, cfg)

	mutatingEndpoints := []struct {
		path string
		body string
	}{
		{"/api/v1/notifications/notif-1/sent", ""},
		{"/api/v1/notifications/notif-1/failed", `{"reason":"timeout"}`},
		{"/api/v1/ai/queue/inc-1/done", `{"incidentId":"inc-1","analysis":"ok","severity":"low"}`},
		{"/api/v1/ai/queue/inc-1/failed", `{"reason":"timeout"}`},
	}

	t.Run("without auth returns 401", func(t *testing.T) {
		for _, ep := range mutatingEndpoints {
			var body *strings.Reader
			if ep.body != "" {
				body = strings.NewReader(ep.body)
			} else {
				body = strings.NewReader("")
			}
			req := httptest.NewRequest(http.MethodPost, ep.path, body)
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			if strings.HasPrefix(ep.path, "/api/v1/notifications/") {
				h.handleNotificationByID(rec, req)
			} else {
				h.handleAIQueueByID(rec, req)
			}

			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s: expected 401, got %d", ep.path, rec.Code)
			}
		}
	})

	t.Run("with auth succeeds", func(t *testing.T) {
		for _, ep := range mutatingEndpoints {
			var body *strings.Reader
			if ep.body != "" {
				body = strings.NewReader(ep.body)
			} else {
				body = strings.NewReader("")
			}
			req := httptest.NewRequest(http.MethodPost, ep.path, body)
			req.Header.Set("Content-Type", "application/json")
			req.SetBasicAuth("admin", "pass")
			rec := httptest.NewRecorder()

			if strings.HasPrefix(ep.path, "/api/v1/notifications/") {
				h.handleNotificationByID(rec, req)
			} else {
				h.handleAIQueueByID(rec, req)
			}

			// Should not be 401 — could be 200 or a domain error, but not auth failure
			if rec.Code == http.StatusUnauthorized {
				t.Errorf("%s: got 401 with valid credentials", ep.path)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Part 2: Security Tests (spec Gate G7)
// ---------------------------------------------------------------------------

func TestSecurityNoSecretsInAPIResponses(t *testing.T) {
	const secret = "secretpassword"
	const dsn = "user:" + secret + "@tcp(localhost:3306)/testdb"
	os.Setenv("MYSQL_TEST_DSN", dsn)
	defer os.Unsetenv("MYSQL_TEST_DSN")

	dir := t.TempDir()
	repo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	now := time.Now().UTC()
	s1 := MySQLSample{SampleID: "s1", CheckID: "test-db", Timestamp: now, Connections: 100, MaxConnections: 200}
	s2 := MySQLSample{SampleID: "s2", CheckID: "test-db", Timestamp: now.Add(time.Minute), Connections: 110, MaxConnections: 200}
	repo.AppendSample(s1)
	id2, _ := repo.AppendSample(s2)
	repo.ComputeAndAppendDelta(id2)

	cfg := &Config{}
	h := NewMySQLAPIHandler(repo, nil, nil, nil, nil, cfg)

	// Check samples endpoint
	req := httptest.NewRequest(http.MethodGet, "/api/v1/mysql/samples?checkId=test-db", nil)
	rec := httptest.NewRecorder()
	h.handleMySQLSamples(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, secret) {
		t.Error("samples response contains secret password")
	}
	if strings.Contains(body, dsn) {
		t.Error("samples response contains full DSN")
	}

	// Check deltas endpoint
	req = httptest.NewRequest(http.MethodGet, "/api/v1/mysql/deltas?checkId=test-db", nil)
	rec = httptest.NewRecorder()
	h.handleMySQLDeltas(rec, req)

	body = rec.Body.String()
	if strings.Contains(body, secret) {
		t.Error("deltas response contains secret password")
	}
	if strings.Contains(body, dsn) {
		t.Error("deltas response contains full DSN")
	}
}

func TestSecurityMySQLDSNRedaction(t *testing.T) {
	const dsnValue = "root:s3cret@tcp(db.internal:3306)/prod"
	os.Setenv("TEST_DSN_ENV", dsnValue)
	defer os.Unsetenv("TEST_DSN_ENV")

	checkCfg := CheckConfig{
		ID:   "mysql-1",
		Name: "Production MySQL",
		Type: "mysql",
		MySQL: &MySQLCheckConfig{
			DSNEnv: "TEST_DSN_ENV",
		},
	}

	// The config field should reference the env var name, not the actual DSN
	cfgJSON, _ := json.Marshal(checkCfg)
	cfgStr := string(cfgJSON)

	if !strings.Contains(cfgStr, "TEST_DSN_ENV") {
		t.Error("config JSON should contain the env var name")
	}
	if strings.Contains(cfgStr, dsnValue) {
		t.Error("config JSON must NOT contain the actual DSN value")
	}
	if strings.Contains(cfgStr, "s3cret") {
		t.Error("config JSON must NOT contain the DSN password")
	}
}

func TestSecurityAllMutatingEndpointsRequireAuth(t *testing.T) {
	dir := t.TempDir()
	outboxPath := filepath.Join(dir, "notifications.jsonl")
	outbox, err := NewFileNotificationOutbox(outboxPath)
	if err != nil {
		t.Fatalf("create outbox: %v", err)
	}

	aiQueue, err := NewFileAIQueue(dir)
	if err != nil {
		t.Fatalf("create ai queue: %v", err)
	}

	cfg := &Config{
		Auth: AuthConfig{Enabled: true, Username: "admin", Password: "pass"},
	}
	h := NewMySQLAPIHandler(nil, nil, outbox, aiQueue, nil, cfg)

	endpoints := []struct {
		path    string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{"/api/v1/notifications/notif-1/sent", h.handleNotificationByID},
		{"/api/v1/notifications/notif-1/failed", h.handleNotificationByID},
		{"/api/v1/ai/queue/inc-1/done", h.handleAIQueueByID},
		{"/api/v1/ai/queue/inc-1/failed", h.handleAIQueueByID},
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(http.MethodPost, ep.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		ep.handler(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: expected 401 without auth, got %d", ep.path, rec.Code)
		}
	}
}

func TestSecurityInputValidation(t *testing.T) {
	dir := t.TempDir()
	repo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	outboxPath := filepath.Join(dir, "notifications.jsonl")
	outbox, err := NewFileNotificationOutbox(outboxPath)
	if err != nil {
		t.Fatalf("create outbox: %v", err)
	}

	aiQueue, err := NewFileAIQueue(dir)
	if err != nil {
		t.Fatalf("create ai queue: %v", err)
	}

	cfg := &Config{} // auth disabled so we can test input validation paths
	h := NewMySQLAPIHandler(repo, nil, outbox, aiQueue, nil, cfg)

	t.Run("samples with empty checkId returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/mysql/samples?checkId=", nil)
		rec := httptest.NewRecorder()
		h.handleMySQLSamples(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("samples with whitespace checkId returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/mysql/samples?checkId=+", nil)
		rec := httptest.NewRecorder()
		h.handleMySQLSamples(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("deltas with empty checkId returns 400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/mysql/deltas?checkId=", nil)
		rec := httptest.NewRecorder()
		h.handleMySQLDeltas(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("notification failed with invalid JSON body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/notif-1/failed", strings.NewReader("not-json"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.handleNotificationByID(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("ai queue done with invalid JSON body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/queue/inc-1/done", strings.NewReader("not-json"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.handleAIQueueByID(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("ai queue failed with invalid JSON body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/queue/inc-1/failed", strings.NewReader("not-json"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.handleAIQueueByID(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})
}
