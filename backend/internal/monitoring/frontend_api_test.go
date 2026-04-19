package monitoring

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testStore is a minimal Store implementation for frontend API testing.
type testStore struct {
	snapshot State
}

func (s *testStore) Snapshot() State                          { return cloneState(s.snapshot) }
func (s *testStore) DashboardSnapshot() DashboardSnapshot     { return buildDashboardSnapshot(s.snapshot) }
func (s *testStore) Update(fn func(*State) error) error       { return fn(&s.snapshot) }
func (s *testStore) ReplaceChecks(checks []CheckConfig) error { s.snapshot.Checks = checks; return nil }
func (s *testStore) AppendResults(results []CheckResult, _ int) error {
	s.snapshot.Results = append(s.snapshot.Results, results...)
	return nil
}
func (s *testStore) SetLastRun(t time.Time) error { s.snapshot.LastRunAt = t; return nil }

func (s *testStore) UpsertCheck(check CheckConfig) error {
	for i, c := range s.snapshot.Checks {
		if c.ID == check.ID {
			s.snapshot.Checks[i] = check
			return nil
		}
	}
	s.snapshot.Checks = append(s.snapshot.Checks, check)
	return nil
}

func (s *testStore) DeleteCheck(id string) error {
	for i, c := range s.snapshot.Checks {
		if c.ID == id {
			s.snapshot.Checks = append(s.snapshot.Checks[:i], s.snapshot.Checks[i+1:]...)
			return nil
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Analytics Tests
// ---------------------------------------------------------------------------

func TestAnalyticsUptimeEndpoint(t *testing.T) {
	now := time.Now().UTC()
	store := &testStore{
		snapshot: State{
			Checks: []CheckConfig{
				{ID: "api-1", Name: "API 1", Type: "api", Target: "https://example.com", Enabled: boolPtr(true)},
				{ID: "api-2", Name: "API 2", Type: "api", Target: "https://two.com", Enabled: boolPtr(true)},
			},
			Results: []CheckResult{
				{ID: "r1", CheckID: "api-1", Name: "API 1", Status: "healthy", Healthy: true, DurationMs: 50, StartedAt: now.Add(-1 * time.Hour), FinishedAt: now.Add(-1 * time.Hour)},
				{ID: "r2", CheckID: "api-1", Name: "API 1", Status: "healthy", Healthy: true, DurationMs: 60, StartedAt: now.Add(-2 * time.Hour), FinishedAt: now.Add(-2 * time.Hour)},
				{ID: "r3", CheckID: "api-1", Name: "API 1", Status: "critical", Healthy: false, DurationMs: 200, StartedAt: now.Add(-3 * time.Hour), FinishedAt: now.Add(-3 * time.Hour)},
				{ID: "r4", CheckID: "api-2", Name: "API 2", Status: "healthy", Healthy: true, DurationMs: 30, StartedAt: now.Add(-1 * time.Hour), FinishedAt: now.Add(-1 * time.Hour)},
				{ID: "r5", CheckID: "api-2", Name: "API 2", Status: "critical", Healthy: false, DurationMs: 300, StartedAt: now.Add(-2 * time.Hour), FinishedAt: now.Add(-2 * time.Hour)},
			},
		},
	}

	service := NewService(&Config{RetentionDays: 7}, store, nil)

	t.Run("single check with period", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/uptime?checkId=api-1&period=7d", nil)
		rec := httptest.NewRecorder()
		service.handleAnalyticsUptime(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var stats UptimeStats
		if err := decodeAPIResponseData(rec.Body.Bytes(), &stats); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if stats.CheckID != "api-1" {
			t.Errorf("expected checkId api-1, got %s", stats.CheckID)
		}
		if stats.TotalResults != 3 {
			t.Errorf("expected 3 results, got %d", stats.TotalResults)
		}
		if stats.HealthyCount != 2 {
			t.Errorf("expected 2 healthy, got %d", stats.HealthyCount)
		}
		// 2/3 = 66.67%
		if stats.UptimePct < 66 || stats.UptimePct > 67 {
			t.Errorf("expected uptimePct ~66.67, got %.2f", stats.UptimePct)
		}
	})

	t.Run("all checks", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/uptime?period=7d", nil)
		rec := httptest.NewRecorder()
		service.handleAnalyticsUptime(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var stats []UptimeStats
		if err := decodeAPIResponseData(rec.Body.Bytes(), &stats); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(stats) != 2 {
			t.Fatalf("expected 2 uptime entries, got %d", len(stats))
		}
	})

	t.Run("missing period defaults to 24h", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/uptime?checkId=api-1", nil)
		rec := httptest.NewRecorder()
		service.handleAnalyticsUptime(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var stats UptimeStats
		if err := decodeAPIResponseData(rec.Body.Bytes(), &stats); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if stats.Period != "24h" {
			t.Errorf("expected period 24h, got %s", stats.Period)
		}
	})
}

func TestAnalyticsResponseTimesEndpoint(t *testing.T) {
	now := time.Now().UTC()
	store := &testStore{
		snapshot: State{
			Checks: []CheckConfig{
				{ID: "api-1", Name: "API 1", Type: "api", Enabled: boolPtr(true)},
			},
			Results: []CheckResult{
				{ID: "r1", CheckID: "api-1", DurationMs: 50, StartedAt: now.Add(-1 * time.Hour)},
				{ID: "r2", CheckID: "api-1", DurationMs: 100, StartedAt: now.Add(-2 * time.Hour)},
				{ID: "r3", CheckID: "api-1", DurationMs: 150, StartedAt: now.Add(-3 * time.Hour)},
				{ID: "r4", CheckID: "api-1", DurationMs: 200, StartedAt: now.Add(-4 * time.Hour)},
			},
		},
	}

	service := NewService(&Config{RetentionDays: 7}, store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/response-times?checkId=api-1&period=24h&interval=1h", nil)
	rec := httptest.NewRecorder()
	service.handleAnalyticsResponseTimes(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var buckets []ResponseTimeBucket
	if err := decodeAPIResponseData(rec.Body.Bytes(), &buckets); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(buckets) == 0 {
		t.Fatal("expected at least 1 response time bucket")
	}
}

func TestAnalyticsFailureRateEndpoint(t *testing.T) {
	now := time.Now().UTC()
	store := &testStore{
		snapshot: State{
			Checks: []CheckConfig{
				{ID: "api-1", Name: "API 1", Type: "api", Server: "prod-1", Enabled: boolPtr(true)},
				{ID: "api-2", Name: "API 2", Type: "api", Server: "prod-2", Enabled: boolPtr(true)},
			},
			Results: []CheckResult{
				{ID: "r1", CheckID: "api-1", Server: "prod-1", Status: "healthy", Healthy: true, StartedAt: now.Add(-1 * time.Hour)},
				{ID: "r2", CheckID: "api-1", Server: "prod-1", Status: "critical", Healthy: false, StartedAt: now.Add(-2 * time.Hour)},
				{ID: "r3", CheckID: "api-2", Server: "prod-2", Status: "healthy", Healthy: true, StartedAt: now.Add(-1 * time.Hour)},
				{ID: "r4", CheckID: "api-2", Server: "prod-2", Status: "healthy", Healthy: true, StartedAt: now.Add(-2 * time.Hour)},
			},
		},
	}

	service := NewService(&Config{RetentionDays: 7}, store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/failure-rate?period=7d&groupBy=server", nil)
	rec := httptest.NewRecorder()
	service.handleAnalyticsFailureRate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var entries []FailureRateEntry
	if err := decodeAPIResponseData(rec.Body.Bytes(), &entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(entries))
	}
	// prod-1 should have 50% failure rate (1 out of 2)
	for _, entry := range entries {
		if entry.Group == "prod-1" {
			if entry.FailureRate != 50.0 {
				t.Errorf("expected prod-1 failure rate 50%%, got %.2f%%", entry.FailureRate)
			}
		}
		if entry.Group == "prod-2" {
			if entry.FailureRate != 0.0 {
				t.Errorf("expected prod-2 failure rate 0%%, got %.2f%%", entry.FailureRate)
			}
		}
	}
}

func TestStatsOverviewEndpoint(t *testing.T) {
	now := time.Now().UTC()
	logger := newTestLogger()

	store := &testStore{
		snapshot: State{
			Checks: []CheckConfig{
				{ID: "api-1", Name: "API 1", Type: "api", Server: "prod-1", Enabled: boolPtr(true)},
				{ID: "tcp-1", Name: "TCP 1", Type: "tcp", Server: "prod-1", Enabled: boolPtr(true)},
				{ID: "proc-1", Name: "Proc 1", Type: "process", Server: "prod-2", Enabled: boolPtr(false)},
			},
			Results: []CheckResult{
				{ID: "r1", CheckID: "api-1", Status: "healthy", Healthy: true, StartedAt: now.Add(-1 * time.Hour)},
				{ID: "r2", CheckID: "tcp-1", Status: "critical", Healthy: false, StartedAt: now.Add(-1 * time.Hour)},
			},
		},
	}

	service := NewService(&Config{RetentionDays: 7}, store, logger)

	incidentRepo := NewMemoryIncidentRepository()
	incidentManager := NewIncidentManager(incidentRepo, logger)
	service.SetIncidentManager(incidentManager)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/stats/overview", nil)
	rec := httptest.NewRecorder()
	service.handleStatsOverview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var overview OverviewStats
	if err := decodeAPIResponseData(rec.Body.Bytes(), &overview); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if overview.TotalChecks != 3 {
		t.Errorf("expected totalChecks=3, got %d", overview.TotalChecks)
	}
	if overview.EnabledChecks != 2 {
		t.Errorf("expected enabledChecks=2, got %d", overview.EnabledChecks)
	}
	if overview.ChecksByType["api"] != 1 {
		t.Errorf("expected checksByType[api]=1, got %d", overview.ChecksByType["api"])
	}
	if overview.ChecksByType["tcp"] != 1 {
		t.Errorf("expected checksByType[tcp]=1, got %d", overview.ChecksByType["tcp"])
	}
	if overview.ChecksByType["process"] != 1 {
		t.Errorf("expected checksByType[process]=1, got %d", overview.ChecksByType["process"])
	}
}

func TestCheckDetailEndpoint(t *testing.T) {
	now := time.Now().UTC()
	logger := newTestLogger()

	store := &testStore{
		snapshot: State{
			Checks: []CheckConfig{
				{ID: "api-1", Name: "API 1", Type: "api", Target: "https://example.com", Enabled: boolPtr(true)},
			},
			Results: []CheckResult{
				{ID: "r1", CheckID: "api-1", Name: "API 1", Status: "healthy", Healthy: true, DurationMs: 50, StartedAt: now.Add(-1 * time.Hour)},
				{ID: "r2", CheckID: "api-1", Name: "API 1", Status: "critical", Healthy: false, DurationMs: 200, StartedAt: now.Add(-2 * time.Hour)},
				{ID: "r3", CheckID: "api-1", Name: "API 1", Status: "healthy", Healthy: true, DurationMs: 80, StartedAt: now.Add(-3 * time.Hour)},
			},
		},
	}

	service := NewService(&Config{RetentionDays: 7}, store, logger)

	incidentRepo := NewMemoryIncidentRepository()
	incidentManager := NewIncidentManager(incidentRepo, logger)
	service.SetIncidentManager(incidentManager)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/checks/api-1", nil)
	rec := httptest.NewRecorder()
	service.handleGetCheck(rec, req, "api-1")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var detail CheckDetail
	if err := decodeAPIResponseData(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.Config.ID != "api-1" {
		t.Errorf("expected config.id=api-1, got %s", detail.Config.ID)
	}
	if detail.LatestResult == nil {
		t.Fatal("expected latestResult to be set")
	}
	if len(detail.RecentResults) != 3 {
		t.Errorf("expected 3 recentResults, got %d", len(detail.RecentResults))
	}
}

// ---------------------------------------------------------------------------
// Config Tests
// ---------------------------------------------------------------------------

func TestConfigGetEndpoint(t *testing.T) {
	cfg := &Config{
		Server:               ServerConfig{Addr: ":8080"},
		Auth:                 AuthConfig{Enabled: true, Username: "admin", Password: "supersecret"},
		RetentionDays:        7,
		CheckIntervalSeconds: 60,
		Workers:              4,
	}
	service := NewService(cfg, &testStore{}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	service.handleConfig(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var view SafeConfigView
	if err := decodeAPIResponseData(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.RetentionDays != 7 {
		t.Errorf("expected retentionDays=7, got %d", view.RetentionDays)
	}
	if view.Workers != 4 {
		t.Errorf("expected workers=4, got %d", view.Workers)
	}
	if !view.AuthEnabled {
		t.Error("expected authEnabled=true")
	}

	// Ensure no credentials leaked in response body
	body := rec.Body.String()
	if strings.Contains(body, "supersecret") {
		t.Error("response contains auth password — credentials leaked!")
	}
	if strings.Contains(body, `"username"`) {
		t.Error("response contains username field — credentials may be leaked")
	}
}

func TestConfigUpdateEndpoint(t *testing.T) {
	cfg := &Config{
		Auth:          AuthConfig{Enabled: true, Username: "admin", Password: "secret"},
		RetentionDays: 7,
	}
	service := NewService(cfg, &testStore{}, nil)

	t.Run("PUT without auth returns 401", func(t *testing.T) {
		body := `{"retentionDays": 14}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/config", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		service.handleConfig(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("PUT with auth and valid body", func(t *testing.T) {
		body := `{"retentionDays": 14}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/config", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "secret")
		rec := httptest.NewRecorder()
		service.handleConfig(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
		if cfg.RetentionDays != 14 {
			t.Errorf("expected config updated to 14, got %d", cfg.RetentionDays)
		}
	})

	t.Run("PUT with invalid retention 0", func(t *testing.T) {
		body := `{"retentionDays": 0}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/config", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "secret")
		rec := httptest.NewRecorder()
		service.handleConfig(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for retentionDays=0, got %d", rec.Code)
		}
	})

	t.Run("PUT with invalid retention 999", func(t *testing.T) {
		body := `{"retentionDays": 999}`
		req := httptest.NewRequest(http.MethodPut, "/api/v1/config", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "secret")
		rec := httptest.NewRecorder()
		service.handleConfig(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 for retentionDays=999, got %d", rec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Alert Rules Tests
// ---------------------------------------------------------------------------

func TestAlertRulesEndpoint(t *testing.T) {
	logger := newTestLogger()
	cfg := &Config{
		Auth: AuthConfig{Enabled: true, Username: "admin", Password: "secret"},
	}
	service := NewService(cfg, &testStore{}, logger)

	rules := []AlertRule{
		{ID: "r1", Name: "Rule 1", Enabled: true, Severity: "critical"},
		{ID: "r2", Name: "Rule 2", Enabled: true, Severity: "warning"},
	}
	alertEngine := NewAlertRuleEngine(rules, logger)
	service.SetAlertEngine(alertEngine)

	t.Run("GET returns initial rules", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/alert-rules", nil)
		rec := httptest.NewRecorder()
		service.handleAlertRules(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var got []AlertRule
		if err := decodeAPIResponseData(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 rules, got %d", len(got))
		}
	})

	t.Run("POST with auth creates rule", func(t *testing.T) {
		newRule := AlertRule{ID: "r3", Name: "Rule 3", Severity: "critical", Enabled: true}
		body := mustMarshalJSON(t, newRule)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/alert-rules", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.SetBasicAuth("admin", "secret")
		rec := httptest.NewRecorder()
		service.handleAlertRules(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("GET returns 3 rules after create", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/alert-rules", nil)
		rec := httptest.NewRecorder()
		service.handleAlertRules(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var got []AlertRule
		if err := decodeAPIResponseData(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if len(got) != 3 {
			t.Fatalf("expected 3 rules after create, got %d", len(got))
		}
	})

	t.Run("POST without auth returns 401", func(t *testing.T) {
		newRule := AlertRule{ID: "r4", Name: "Rule 4", Severity: "warning"}
		body := mustMarshalJSON(t, newRule)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/alert-rules", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		service.handleAlertRules(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}

func TestAlertRuleByIDEndpoint(t *testing.T) {
	logger := newTestLogger()
	cfg := &Config{
		Auth: AuthConfig{Enabled: true, Username: "admin", Password: "secret"},
	}
	service := NewService(cfg, &testStore{}, logger)

	rules := []AlertRule{
		{ID: "r1", Name: "Rule 1", Enabled: true, Severity: "critical"},
	}
	alertEngine := NewAlertRuleEngine(rules, logger)
	service.SetAlertEngine(alertEngine)

	t.Run("DELETE with auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/alert-rules/r1", nil)
		req.SetBasicAuth("admin", "secret")
		rec := httptest.NewRecorder()
		service.handleAlertRuleByID(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("DELETE without auth returns 401", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/alert-rules/r1", nil)
		rec := httptest.NewRecorder()
		service.handleAlertRuleByID(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})

	t.Run("DELETE non-existent returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/alert-rules/no-such-rule", nil)
		req.SetBasicAuth("admin", "secret")
		rec := httptest.NewRecorder()
		service.handleAlertRuleByID(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", rec.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// Incident Filter Tests
// ---------------------------------------------------------------------------

func TestIncidentFilterEndpoint(t *testing.T) {
	logger := newTestLogger()
	cfg := &Config{RetentionDays: 7}
	service := NewService(cfg, &testStore{}, logger)

	incidentRepo := NewMemoryIncidentRepository()
	incidentManager := NewIncidentManager(incidentRepo, logger)
	service.SetIncidentManager(incidentManager)

	now := time.Now().UTC()

	// 1 open+critical, 1 open+warning, 1 resolved+critical
	_ = incidentRepo.CreateIncident(Incident{
		ID: "inc-1", CheckID: "c1", Status: "open", Severity: "critical", StartedAt: now,
	})
	_ = incidentRepo.CreateIncident(Incident{
		ID: "inc-2", CheckID: "c2", Status: "open", Severity: "warning", StartedAt: now,
	})
	resolvedAt := now.Add(-1 * time.Hour)
	_ = incidentRepo.CreateIncident(Incident{
		ID: "inc-3", CheckID: "c3", Status: "resolved", Severity: "critical", StartedAt: now.Add(-2 * time.Hour), ResolvedAt: &resolvedAt,
	})

	t.Run("filter status=open returns 2", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents?status=open", nil)
		rec := httptest.NewRecorder()
		service.handleIncidentsFiltered(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var resp APIResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}

		// Data is a PaginatedResponse
		dataBytes, _ := json.Marshal(resp.Data)
		var paginated PaginatedResponse
		if err := json.Unmarshal(dataBytes, &paginated); err != nil {
			t.Fatalf("decode paginated: %v", err)
		}
		if paginated.Total != 2 {
			t.Errorf("expected total=2, got %d", paginated.Total)
		}
	})

	t.Run("filter status=open&severity=critical returns 1", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents?status=open&severity=critical", nil)
		rec := httptest.NewRecorder()
		service.handleIncidentsFiltered(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var resp APIResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		dataBytes, _ := json.Marshal(resp.Data)
		var paginated PaginatedResponse
		_ = json.Unmarshal(dataBytes, &paginated)
		if paginated.Total != 1 {
			t.Errorf("expected total=1, got %d", paginated.Total)
		}
	})

	t.Run("pagination limit=1 offset=0", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents?limit=1&offset=0", nil)
		rec := httptest.NewRecorder()
		service.handleIncidentsFiltered(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var resp APIResponse
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		dataBytes, _ := json.Marshal(resp.Data)
		var paginated PaginatedResponse
		_ = json.Unmarshal(dataBytes, &paginated)
		if paginated.Total != 3 {
			t.Errorf("expected total=3, got %d", paginated.Total)
		}
		if paginated.Limit != 1 {
			t.Errorf("expected limit=1, got %d", paginated.Limit)
		}

		// Items should have only 1 entry
		itemsBytes, _ := json.Marshal(paginated.Items)
		var items []Incident
		_ = json.Unmarshal(itemsBytes, &items)
		if len(items) != 1 {
			t.Errorf("expected 1 item on page, got %d", len(items))
		}
	})
}

// ---------------------------------------------------------------------------
// SSE Test
// ---------------------------------------------------------------------------

func TestSSEEndpoint(t *testing.T) {
	store := &testStore{
		snapshot: State{
			Checks: []CheckConfig{
				{ID: "api-1", Name: "API 1", Type: "api", Enabled: boolPtr(true)},
			},
		},
	}
	service := NewService(&Config{RetentionDays: 7}, store, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	service.handleSSE(rec, req)

	contentType := rec.Header().Get("Content-Type")
	if contentType != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", contentType)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "data:") {
		t.Error("expected at least one data: line in SSE output")
	}
}

// ---------------------------------------------------------------------------
// MySQL Analytics Tests
// ---------------------------------------------------------------------------

func TestMySQLHealthSummaryEndpoint(t *testing.T) {
	dir := t.TempDir()

	repo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	now := time.Now().UTC()
	_, err = repo.AppendSample(MySQLSample{
		SampleID:       "s1",
		CheckID:        "test-db",
		Timestamp:      now,
		Connections:    800,
		MaxConnections: 1000,
	})
	if err != nil {
		t.Fatalf("append sample: %v", err)
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

	cfg := &Config{}
	handler := NewMySQLAPIHandler(repo, snapshotRepo, outbox, aiQueue, nil, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mysql/health?checkId=test-db", nil)
	rec := httptest.NewRecorder()
	handler.handleMySQLHealthSummary(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var summary MySQLHealthSummary
	if err := decodeAPIResponseData(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if summary.ConnectionUtilPct != 80.0 {
		t.Errorf("expected connectionUtilPct=80.0, got %.2f", summary.ConnectionUtilPct)
	}
	if summary.Status != "warning" {
		t.Errorf("expected status=warning (>70%%), got %s", summary.Status)
	}
}

func TestMySQLTimeSeriesEndpoint(t *testing.T) {
	dir := t.TempDir()

	repo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		_, err = repo.AppendSample(MySQLSample{
			SampleID:       "s" + string(rune('1'+i)),
			CheckID:        "test-db",
			Timestamp:      now.Add(-time.Duration(4-i) * time.Minute),
			Connections:    int64(100 + i*10),
			MaxConnections: 1000,
			ThreadsRunning: int64(5 + i),
		})
		if err != nil {
			t.Fatalf("append sample %d: %v", i, err)
		}
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

	cfg := &Config{}
	handler := NewMySQLAPIHandler(repo, snapshotRepo, outbox, aiQueue, nil, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/mysql/timeseries?checkId=test-db&limit=5", nil)
	rec := httptest.NewRecorder()
	handler.handleMySQLTimeSeries(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var points []MySQLTimeSeriesPoint
	if err := decodeAPIResponseData(rec.Body.Bytes(), &points); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(points) != 5 {
		t.Fatalf("expected 5 time series points, got %d", len(points))
	}
	// Should be sorted ascending
	for i := 1; i < len(points); i++ {
		if points[i].Timestamp.Before(points[i-1].Timestamp) {
			t.Error("time series points not sorted ascending by timestamp")
			break
		}
	}
}

func TestNotificationStatsEndpoint(t *testing.T) {
	dir := t.TempDir()

	outbox, err := NewFileNotificationOutbox(filepath.Join(dir, "outbox.jsonl"))
	if err != nil {
		t.Fatalf("create outbox: %v", err)
	}

	// Enqueue 3 notifications
	_ = outbox.Enqueue(NotificationEvent{NotificationID: "n1", IncidentID: "inc-1", Channel: "slack"})
	_ = outbox.Enqueue(NotificationEvent{NotificationID: "n2", IncidentID: "inc-2", Channel: "email"})
	_ = outbox.Enqueue(NotificationEvent{NotificationID: "n3", IncidentID: "inc-3", Channel: "slack"})

	// Mark 1 sent, 1 failed
	_ = outbox.MarkSent("n2")
	_ = outbox.MarkFailed("n3", "timeout")

	repo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	snapshotRepo, err := NewFileSnapshotRepository(filepath.Join(dir, "snapshots.jsonl"))
	if err != nil {
		t.Fatalf("create snapshot repo: %v", err)
	}
	aiQueue, err := NewFileAIQueue(dir)
	if err != nil {
		t.Fatalf("create ai queue: %v", err)
	}

	cfg := &Config{}
	handler := NewMySQLAPIHandler(repo, snapshotRepo, outbox, aiQueue, nil, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/stats", nil)
	rec := httptest.NewRecorder()
	handler.handleNotificationStats(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var stats NotificationStats
	if err := decodeAPIResponseData(rec.Body.Bytes(), &stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stats.Total != 3 {
		t.Errorf("expected total=3, got %d", stats.Total)
	}
	if stats.Pending != 1 {
		t.Errorf("expected pending=1, got %d", stats.Pending)
	}
	if stats.Sent != 1 {
		t.Errorf("expected sent=1, got %d", stats.Sent)
	}
	if stats.Failed != 1 {
		t.Errorf("expected failed=1, got %d", stats.Failed)
	}
}

// ---------------------------------------------------------------------------
// Export Tests
// ---------------------------------------------------------------------------

func TestExportSamplesCSV(t *testing.T) {
	dir := t.TempDir()

	repo, err := NewFileMySQLRepository(dir)
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		_, err = repo.AppendSample(MySQLSample{
			SampleID:       "s" + string(rune('1'+i)),
			CheckID:        "test-db",
			Timestamp:      now.Add(-time.Duration(2-i) * time.Minute),
			Connections:    int64(50 + i*10),
			MaxConnections: 200,
		})
		if err != nil {
			t.Fatalf("append sample: %v", err)
		}
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

	cfg := &Config{}
	handler := NewMySQLAPIHandler(repo, snapshotRepo, outbox, aiQueue, nil, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/mysql/samples?checkId=test-db&format=csv", nil)
	rec := httptest.NewRecorder()
	handler.handleExportSamples(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "text/csv" {
		t.Errorf("expected Content-Type text/csv, got %s", ct)
	}

	reader := csv.NewReader(rec.Body)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	// 1 header + 3 data rows
	if len(records) != 4 {
		t.Errorf("expected 4 CSV rows (1 header + 3 data), got %d", len(records))
	}
}

func TestExportIncidentsCSV(t *testing.T) {
	incidentRepo := NewMemoryIncidentRepository()

	now := time.Now().UTC()
	_ = incidentRepo.CreateIncident(Incident{
		ID: "inc-1", CheckID: "c1", CheckName: "Check 1", Status: "open", Severity: "critical", StartedAt: now,
	})
	_ = incidentRepo.CreateIncident(Incident{
		ID: "inc-2", CheckID: "c2", CheckName: "Check 2", Status: "resolved", Severity: "warning", StartedAt: now.Add(-1 * time.Hour),
	})

	handler := handleExportIncidents(incidentRepo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/incidents?format=csv", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "text/csv" {
		t.Errorf("expected Content-Type text/csv, got %s", ct)
	}

	reader := csv.NewReader(rec.Body)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	// 1 header + 2 data rows
	if len(records) != 3 {
		t.Errorf("expected 3 CSV rows (1 header + 2 data), got %d", len(records))
	}
}

func TestExportResultsCSV(t *testing.T) {
	now := time.Now().UTC()
	store := &testStore{
		snapshot: State{
			Results: []CheckResult{
				{ID: "r1", CheckID: "api-1", Name: "API 1", Status: "healthy", Healthy: true, DurationMs: 50, StartedAt: now.Add(-1 * time.Hour), FinishedAt: now.Add(-1 * time.Hour)},
				{ID: "r2", CheckID: "api-1", Name: "API 1", Status: "critical", Healthy: false, DurationMs: 200, StartedAt: now.Add(-2 * time.Hour), FinishedAt: now.Add(-2 * time.Hour)},
				{ID: "r3", CheckID: "api-2", Name: "API 2", Status: "healthy", Healthy: true, DurationMs: 30, StartedAt: now.Add(-3 * time.Hour), FinishedAt: now.Add(-3 * time.Hour)},
				{ID: "r4", CheckID: "api-1", Name: "API 1", Status: "healthy", Healthy: true, DurationMs: 40, StartedAt: now.Add(-4 * time.Hour), FinishedAt: now.Add(-4 * time.Hour)},
				{ID: "r5", CheckID: "api-2", Name: "API 2", Status: "warning", Healthy: false, DurationMs: 150, StartedAt: now.Add(-5 * time.Hour), FinishedAt: now.Add(-5 * time.Hour)},
			},
		},
	}

	handler := handleExportResults(store, 7)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/export/results?format=csv", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "text/csv" {
		t.Errorf("expected Content-Type text/csv, got %s", ct)
	}

	reader := csv.NewReader(rec.Body)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	// 1 header + 5 data rows
	if len(records) != 6 {
		t.Errorf("expected 6 CSV rows (1 header + 5 data), got %d", len(records))
	}
}

// ---------------------------------------------------------------------------
// Auth Info Test
// ---------------------------------------------------------------------------

func TestAuthMeEndpoint(t *testing.T) {
	cfg := &Config{
		Auth: AuthConfig{Enabled: true, Username: "admin", Password: "secret"},
	}
	service := NewService(cfg, &testStore{}, nil)

	t.Run("with basic auth returns username", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		req.SetBasicAuth("admin", "secret")
		rec := httptest.NewRecorder()
		service.handleAuthMe(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var info AuthInfo
		if err := decodeAPIResponseData(rec.Body.Bytes(), &info); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if info.Username != "admin" {
			t.Errorf("expected username=admin, got %s", info.Username)
		}
		if !info.AuthEnabled {
			t.Error("expected authEnabled=true")
		}
	})

	t.Run("without auth still works", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		rec := httptest.NewRecorder()
		service.handleAuthMe(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}

		var info AuthInfo
		if err := decodeAPIResponseData(rec.Body.Bytes(), &info); err != nil {
			t.Fatalf("decode: %v", err)
		}
		// Without auth credentials, ExtractActorFromRequest returns "unknown"
		if info.Username != "unknown" {
			t.Errorf("expected username=unknown without auth, got %s", info.Username)
		}
	})
}
