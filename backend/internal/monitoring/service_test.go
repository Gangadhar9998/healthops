package monitoring

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Helper functions for testing API responses

func decodeAPIResponse(body []byte) (APIResponse, error) {
	var resp APIResponse
	err := json.Unmarshal(body, &resp)
	return resp, err
}

func decodeAPIResponseData(body []byte, v interface{}) error {
	var resp APIResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return err
	}
	if !resp.Success {
		return fmt.Errorf("API response indicates failure")
	}
	dataBytes, err := json.Marshal(resp.Data)
	if err != nil {
		return err
	}
	return json.Unmarshal(dataBytes, v)
}

func TestDashboardChecksEndpointReturnsReadModelChecks(t *testing.T) {
	store := &fakeStore{
		snapshot: State{
			Checks: []CheckConfig{{ID: "api-1", Name: "API 1", Type: "api", Target: "https://example.com/health", Enabled: boolPtr(true)}},
		},
	}

	service := NewService(&Config{RetentionDays: 7}, store, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/checks", nil)
	rec := httptest.NewRecorder()

	service.handleDashboardChecks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var items []CheckListItem
	if err := decodeAPIResponseData(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 1 || items[0].ID != "api-1" {
		t.Fatalf("unexpected checks payload: %+v", items)
	}
}

func TestDashboardSummaryEndpointReturnsReadModelSummary(t *testing.T) {
	state := State{
		Checks: []CheckConfig{
			{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com"},
			{ID: "c2", Name: "C2", Type: "api", Target: "https://two.com"},
			{ID: "c3", Name: "C3", Type: "api", Target: "https://three.com"},
		},
		Results: []CheckResult{
			{ID: "r1", CheckID: "c1", Status: "healthy", StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC()},
			{ID: "r2", CheckID: "c2", Status: "warning", StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC()},
			{ID: "r3", CheckID: "c3", Status: "critical", StartedAt: time.Now().UTC(), FinishedAt: time.Now().UTC()},
		},
	}
	store := &fakeStore{
		snapshot: state,
	}

	service := NewService(&Config{RetentionDays: 7}, store, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/summary", nil)
	rec := httptest.NewRecorder()

	service.handleDashboardSummary(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var summary Summary
	if err := decodeAPIResponseData(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if summary.TotalChecks != 3 || summary.Healthy != 1 || summary.Warning != 1 || summary.Critical != 1 {
		t.Fatalf("unexpected summary payload: %+v", summary)
	}
}

func TestDashboardResultsEndpointFiltersReadModelResults(t *testing.T) {
	now := time.Now().UTC()
	state := State{
		Results: []CheckResult{
			{ID: "r1", CheckID: "api-1", Status: "healthy", FinishedAt: now},
			{ID: "r2", CheckID: "api-2", Status: "critical", FinishedAt: now.Add(-8 * 24 * time.Hour)},
		},
	}
	store := &fakeStore{
		snapshot: state,
	}

	service := NewService(&Config{RetentionDays: 7}, store, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/results?checkId=api-1&days=7", nil)
	rec := httptest.NewRecorder()

	service.handleDashboardResults(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var results []CheckResult
	if err := decodeAPIResponseData(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(results) != 1 || results[0].ID != "r1" {
		t.Fatalf("unexpected filtered results: %+v", results)
	}
}

func TestHealthzEndpoint(t *testing.T) {
	store := &fakeStore{}
	service := NewService(&Config{}, store, nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	service.handleHealthz(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var data map[string]string
	if err := decodeAPIResponseData(rec.Body.Bytes(), &data); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if data["status"] != "ok" {
		t.Fatalf("expected status=ok, got %+v", data)
	}
}

func TestReadyzEndpoint(t *testing.T) {
	now := time.Now().UTC()
	state := State{
		Checks:    []CheckConfig{{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com"}},
		LastRunAt: now,
	}
	store := &fakeStore{
		snapshot: state,
	}

	service := NewService(&Config{}, store, nil)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	service.handleReadyz(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var data map[string]interface{}
	if err := decodeAPIResponseData(rec.Body.Bytes(), &data); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if data["status"] != "ready" {
		t.Fatalf("expected status=ready, got %+v", data)
	}
	if data["checks"].(float64) != 1 {
		t.Fatalf("expected checks=1, got %+v", data)
	}
}

func TestChecksListEndpoint(t *testing.T) {
	state := State{
		Checks: []CheckConfig{
			{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com", Enabled: boolPtr(true)},
			{ID: "c2", Name: "C2", Type: "api", Target: "https://two.com", Enabled: boolPtr(true)},
		},
	}
	store := &fakeStore{
		snapshot: state,
	}

	service := NewService(&Config{}, store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/checks", nil)
	rec := httptest.NewRecorder()

	service.handleChecks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var items []CheckListItem
	if err := decodeAPIResponseData(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 checks, got %d", len(items))
	}
}

func TestChecksCreateEndpoint(t *testing.T) {
	store := &fakeStore{}
	service := NewService(&Config{}, store, nil)

	newCheck := CheckConfig{
		Name:   "New Check",
		Type:   "api",
		Target: "https://example.com/health",
	}
	body := mustMarshalJSON(t, newCheck)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/checks", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	service.handleChecks(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var created CheckConfig
	if err := decodeAPIResponseData(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if created.Name != "New Check" {
		t.Fatalf("unexpected name: %s", created.Name)
	}
	if created.ID == "" {
		t.Fatal("expected ID to be generated")
	}
}

func TestChecksCreateEndpointValidates(t *testing.T) {
	store := &fakeStore{}
	service := NewService(&Config{}, store, nil)

	invalidCheck := CheckConfig{
		Name: "Invalid Check",
		Type: "api",
		// Missing target - should fail validation
	}
	body := mustMarshalJSON(t, invalidCheck)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/checks", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	service.handleChecks(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var resp APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Success {
		t.Fatal("expected failure response")
	}
	if resp.Error == nil || resp.Error.Message == "" {
		t.Fatal("expected error message")
	}
}

func TestSummaryEndpoint(t *testing.T) {
	now := time.Now().UTC()
	state := State{
		Checks: []CheckConfig{
			{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com"},
		},
		Results: []CheckResult{
			{ID: "r1", CheckID: "c1", Status: "healthy", StartedAt: now, FinishedAt: now},
		},
		LastRunAt: now,
	}
	store := &fakeStore{
		snapshot: state,
	}

	service := NewService(&Config{RetentionDays: 7}, store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/summary", nil)
	rec := httptest.NewRecorder()

	service.handleSummary(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var summary Summary
	if err := decodeAPIResponseData(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if summary.TotalChecks != 1 {
		t.Fatalf("expected TotalChecks=1, got %d", summary.TotalChecks)
	}
	if summary.Healthy != 1 {
		t.Fatalf("expected Healthy=1, got %d", summary.Healthy)
	}
}

func TestResultsEndpointFiltersByCheckId(t *testing.T) {
	now := time.Now().UTC()
	state := State{
		Results: []CheckResult{
			{ID: "r1", CheckID: "c1", Status: "healthy", FinishedAt: now},
			{ID: "r2", CheckID: "c2", Status: "healthy", FinishedAt: now},
		},
	}
	store := &fakeStore{
		snapshot: state,
	}

	service := NewService(&Config{RetentionDays: 7}, store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/results?checkId=c1", nil)
	rec := httptest.NewRecorder()

	service.handleResults(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var results []CheckResult
	if err := decodeAPIResponseData(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(results) != 1 || results[0].CheckID != "c1" {
		t.Fatalf("expected 1 result for c1, got %+v", results)
	}
}

func TestResultsEndpointFiltersByDays(t *testing.T) {
	now := time.Now().UTC()
	state := State{
		Results: []CheckResult{
			{ID: "r1", CheckID: "c1", Status: "healthy", FinishedAt: now.Add(-1 * 24 * time.Hour)},
			{ID: "r2", CheckID: "c1", Status: "healthy", FinishedAt: now.Add(-8 * 24 * time.Hour)},
		},
	}
	store := &fakeStore{
		snapshot: state,
	}

	service := NewService(&Config{RetentionDays: 7}, store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/results?days=7", nil)
	rec := httptest.NewRecorder()

	service.handleResults(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var results []CheckResult
	if err := decodeAPIResponseData(rec.Body.Bytes(), &results); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	// Only r1 should be included (within 7 days)
	if len(results) != 1 || results[0].ID != "r1" {
		t.Fatalf("expected 1 recent result, got %+v", results)
	}
}

func TestRunEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	state := State{
		Checks: []CheckConfig{
			{ID: "c1", Name: "C1", Type: "api", Target: server.URL, Enabled: boolPtr(true)},
		},
	}
	store := &fakeStore{
		snapshot:   state,
		updateFunc: func(s *State) error { return nil },
	}

	cfg := &Config{Workers: 1, RetentionDays: 7}
	service := NewService(cfg, store, nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runs", nil)
	rec := httptest.NewRecorder()

	service.handleRun(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}

	var summary RunSummary
	if err := decodeAPIResponseData(rec.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(summary.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(summary.Results))
	}
}

func TestQueryInt(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/results?days=5", nil)

	// Test valid query
	val := QueryInt(req, "days", 7)
	if val != 5 {
		t.Errorf("expected 5, got %d", val)
	}

	// Test missing query (uses fallback)
	val2 := QueryInt(req, "retention", 14)
	if val2 != 14 {
		t.Errorf("expected fallback 14, got %d", val2)
	}

	// Test invalid query (uses fallback)
	req3 := httptest.NewRequest(http.MethodGet, "/api/v1/results?days=invalid", nil)
	val3 := QueryInt(req3, "days", 7)
	if val3 != 7 {
		t.Errorf("expected fallback 7 for invalid, got %d", val3)
	}
}

// Test new API response format

func TestAPIResponseFormat(t *testing.T) {
	store := &fakeStore{
		snapshot: State{
			Checks: []CheckConfig{
				{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com", Enabled: boolPtr(true)},
			},
		},
	}
	service := NewService(&Config{}, store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/checks", nil)
	rec := httptest.NewRecorder()

	service.handleChecks(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Verify the response envelope
	var resp APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if !resp.Success {
		t.Fatal("expected success=true")
	}
	if resp.Error != nil {
		t.Fatalf("expected no error, got: %v", resp.Error)
	}
	if resp.Data == nil {
		t.Fatal("expected data to be present")
	}

	// Verify the data contains the check list items
	dataBytes, _ := json.Marshal(resp.Data)
	var items []CheckListItem
	if err := json.Unmarshal(dataBytes, &items); err != nil {
		t.Fatalf("decode check items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].ID != "c1" {
		t.Fatalf("expected item id=c1, got %s", items[0].ID)
	}
}

func TestAPIErrorFormat(t *testing.T) {
	store := &fakeStore{}
	service := NewService(&Config{}, store, nil)

	invalidCheck := CheckConfig{
		Name: "Invalid",
		Type: "api",
		// Missing target
	}
	body := mustMarshalJSON(t, invalidCheck)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/checks", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	service.handleChecks(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	var resp APIResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Success {
		t.Fatal("expected success=false for error response")
	}
	if resp.Error == nil {
		t.Fatal("expected error to be present")
	}
	if resp.Error.Code != http.StatusBadRequest {
		t.Fatalf("expected error code 400, got %d", resp.Error.Code)
	}
	if resp.Error.Message == "" {
		t.Fatal("expected error message")
	}
}

func TestCheckListItemConversion(t *testing.T) {
	check := CheckConfig{
		ID:          "test-1",
		Name:        "Test Check",
		Type:        "api",
		Server:      "server-1",
		Application: "app-1",
		Enabled:     boolPtr(true),
		Tags:        []string{"tag1", "tag2"},
	}

	item := toCheckListItem(check)

	if item.ID != check.ID {
		t.Errorf("expected ID %s, got %s", check.ID, item.ID)
	}
	if item.Name != check.Name {
		t.Errorf("expected Name %s, got %s", check.Name, item.Name)
	}
	if item.Type != check.Type {
		t.Errorf("expected Type %s, got %s", check.Type, item.Type)
	}
	if item.Server != check.Server {
		t.Errorf("expected Server %s, got %s", check.Server, item.Server)
	}
	if item.Application != check.Application {
		t.Errorf("expected Application %s, got %s", check.Application, item.Application)
	}
	if !item.Enabled {
		t.Error("expected Enabled=true")
	}
	if len(item.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(item.Tags))
	}
}

func TestToCheckListItems(t *testing.T) {
	checks := []CheckConfig{
		{ID: "c1", Name: "C1", Type: "api", Target: "https://one.com", Enabled: boolPtr(true)},
		{ID: "c2", Name: "C2", Type: "tcp", Host: "example.com", Port: 80, Enabled: boolPtr(false)},
	}

	items := toCheckListItems(checks)

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].ID != "c1" {
		t.Errorf("expected first item id=c1, got %s", items[0].ID)
	}
	if items[1].ID != "c2" {
		t.Errorf("expected second item id=c2, got %s", items[1].ID)
	}
	if !items[0].Enabled {
		t.Error("expected first item to be enabled")
	}
	if items[1].Enabled {
		t.Error("expected second item to be disabled")
	}
}
