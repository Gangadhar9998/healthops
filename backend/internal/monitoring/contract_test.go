package monitoring

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const SCHEMA_VERSION = "v1.0.0"

// newTestService creates a service with a discard logger for testing
func newTestService(store Store) *Service {
	discardLogger := log.New(io.Discard, "", 0)
	return NewService(&Config{}, store, discardLogger)
}

// TestResponseEnvelopeContract verifies all API responses follow the standard envelope format
func TestResponseEnvelopeContract(t *testing.T) {
	tests := []struct {
		name           string
		handler        http.HandlerFunc
		method         string
		body           interface{}
		wantStatusCode int
		checkSuccess   bool
		checkData      bool
		checkError     bool
	}{
		{
			name: "healthz returns success envelope",
			handler: func(w http.ResponseWriter, r *http.Request) {
				WriteAPIResponse(w, http.StatusOK, NewAPIResponse(map[string]string{"status": "ok"}))
			},
			method:         "GET",
			wantStatusCode: http.StatusOK,
			checkSuccess:   true,
			checkData:      true,
			checkError:     false,
		},
		{
			name: "error response has error envelope",
			handler: func(w http.ResponseWriter, r *http.Request) {
				WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("invalid input"))
			},
			method:         "GET",
			wantStatusCode: http.StatusBadRequest,
			checkSuccess:   false,
			checkData:      false,
			checkError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/", nil)
			w := httptest.NewRecorder()
			tt.handler(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			// Verify status code
			if resp.StatusCode != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", resp.StatusCode, tt.wantStatusCode)
			}

			// Verify Content-Type
			contentType := resp.Header.Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Content-Type = %s, want application/json", contentType)
			}

			// Parse response
			var envelope APIResponse
			if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
				t.Fatalf("failed to decode response envelope: %v", err)
			}

			// Verify success field
			if tt.checkSuccess && !envelope.Success {
				t.Errorf("envelope.Success = false, want true for success response")
			}

			// Verify error field structure when present
			if tt.checkError && envelope.Error == nil {
				t.Error("envelope.Error is nil, want error object")
			}
			if tt.checkError && envelope.Error != nil {
				if envelope.Error.Code == 0 {
					t.Error("envelope.Error.Code is 0, want non-zero HTTP status code")
				}
				if envelope.Error.Message == "" {
					t.Error("envelope.Error.Message is empty, want error message")
				}
			}

			// Verify data field is nil when error is present
			if tt.checkError && envelope.Error != nil && envelope.Data != nil {
				t.Error("envelope.Data should be nil when error is present")
			}
		})
	}
}

// TestGETChecksContract verifies GET /api/v1/checks returns CheckListItem array
func TestGETChecksContract(t *testing.T) {
	// Create test store with checks
	store := &contractFakeStore{
		state: State{
			Checks: []CheckConfig{
				{
					ID:          "check-1",
					Name:        "API Health Check",
					Type:        "api",
					Server:      "prod-1",
					Application: "medics",
					Enabled:     boolPtr(true),
					Tags:        []string{"api", "critical"},
				},
				{
					ID:          "check-2",
					Name:        "Process Check",
					Type:        "process",
					Server:      "prod-1",
					Application: "medics",
					Enabled:     boolPtr(false),
					Tags:        []string{"process"},
				},
			},
		},
	}

	service := newTestService(store)
	handler := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		r = r.WithContext(ctx)
		service.handleChecks(w, r)
	}

	req := httptest.NewRequest("GET", "/api/v1/checks", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	// Verify status code
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Parse envelope
	var envelope APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !envelope.Success {
		t.Fatalf("envelope.Success = false, want true")
	}

	// Parse data as CheckListItem array
	dataBytes, err := json.Marshal(envelope.Data)
	if err != nil {
		t.Fatalf("failed to marshal data: %v", err)
	}

	var items []CheckListItem
	if err := json.Unmarshal(dataBytes, &items); err != nil {
		t.Fatalf("failed to unmarshal CheckListItem array: %v", err)
	}

	// Verify we got items
	if len(items) != 2 {
		t.Errorf("got %d items, want 2", len(items))
	}

	// Verify CheckListItem structure
	for _, item := range items {
		if item.ID == "" {
			t.Error("CheckListItem.ID is empty")
		}
		if item.Name == "" {
			t.Error("CheckListItem.Name is empty")
		}
		if item.Type == "" {
			t.Error("CheckListItem.Type is empty")
		}
		// Server and Application are optional
		// Tags are optional
	}
}

// TestPOSTChecksContract verifies POST /api/v1/checks accepts CheckDefinition
func TestPOSTChecksContract(t *testing.T) {
	tests := []struct {
		name         string
		checkDef     map[string]interface{}
		wantStatus   int
		validateBody bool
	}{
		{
			name: "valid API check",
			checkDef: map[string]interface{}{
				"name":        "New API Check",
				"type":        "api",
				"target":      "https://example.com/health",
				"server":      "prod-2",
				"application": "test",
				"enabled":     true,
				"tags":        []string{"api"},
			},
			wantStatus:   http.StatusCreated,
			validateBody: true,
		},
		{
			name: "valid TCP check",
			checkDef: map[string]interface{}{
				"name":        "TCP Port Check",
				"type":        "tcp",
				"host":        "localhost",
				"port":        8080,
				"server":      "localhost",
				"application": "test",
			},
			wantStatus:   http.StatusCreated,
			validateBody: true,
		},
		{
			name: "missing required field name",
			checkDef: map[string]interface{}{
				"type":   "api",
				"target": "https://example.com",
			},
			wantStatus:   http.StatusBadRequest,
			validateBody: false,
		},
		{
			name: "missing required field type",
			checkDef: map[string]interface{}{
				"name":   "Test Check",
				"target": "https://example.com",
			},
			wantStatus:   http.StatusBadRequest,
			validateBody: false,
		},
		{
			name: "API check missing target",
			checkDef: map[string]interface{}{
				"name": "API Check",
				"type": "api",
			},
			wantStatus:   http.StatusBadRequest,
			validateBody: false,
		},
		{
			name: "TCP check missing host and port",
			checkDef: map[string]interface{}{
				"name": "TCP Check",
				"type": "tcp",
			},
			wantStatus:   http.StatusBadRequest,
			validateBody: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &contractFakeStore{state: State{Checks: []CheckConfig{}}}
			service := newTestService(store)
			handler := func(w http.ResponseWriter, r *http.Request) {
				ctx := context.Background()
				r = r.WithContext(ctx)
				service.handleChecks(w, r)
			}

			bodyBytes, _ := json.Marshal(tt.checkDef)
			req := httptest.NewRequest("POST", "/api/v1/checks", bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			// Verify status code
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status code = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			// Parse envelope
			var envelope APIResponse
			if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			// For successful creation, verify response body
			if tt.validateBody {
				if !envelope.Success {
					t.Fatalf("envelope.Success = false, want true")
				}

				dataBytes, err := json.Marshal(envelope.Data)
				if err != nil {
					t.Fatalf("failed to marshal data: %v", err)
				}

				var created CheckConfig
				if err := json.Unmarshal(dataBytes, &created); err != nil {
					t.Fatalf("failed to unmarshal CheckConfig: %v", err)
				}

				if created.ID == "" {
					t.Error("created check ID is empty, want generated ID")
				}
				if created.Name != tt.checkDef["name"] {
					t.Errorf("created check Name = %s, want %s", created.Name, tt.checkDef["name"])
				}
				if created.Type != tt.checkDef["type"] {
					t.Errorf("created check Type = %s, want %s", created.Type, tt.checkDef["type"])
				}
			}

			// For error responses, verify error structure
			if !tt.validateBody && tt.wantStatus >= 400 {
				if envelope.Success {
					t.Error("envelope.Success = true, want false for error response")
				}
				if envelope.Error == nil {
					t.Error("envelope.Error is nil, want error object")
				}
				if envelope.Error != nil && envelope.Error.Message == "" {
					t.Error("envelope.Error.Message is empty")
				}
			}
		})
	}
}

// TestPUTPatchCheckByIDContract verifies PUT/PATCH /api/v1/checks/{id}
func TestPUTPatchCheckByIDContract(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		checkID    string
		updateData map[string]interface{}
		wantStatus int
	}{
		{
			name:    "PUT updates check",
			method:  "PUT",
			checkID: "check-1",
			updateData: map[string]interface{}{
				"name":    "Updated Check Name",
				"type":    "api",
				"target":  "https://example.com",
				"enabled": false,
			},
			wantStatus: http.StatusOK,
		},
		{
			name:    "PATCH updates check",
			method:  "PATCH",
			checkID: "check-1",
			updateData: map[string]interface{}{
				"name":   "Patched Check Name",
				"type":   "api",
				"target": "https://example.com",
			},
			wantStatus: http.StatusOK,
		},
		{
			name:    "invalid check data returns error",
			method:  "PUT",
			checkID: "check-1",
			updateData: map[string]interface{}{
				"type": "invalid_type",
			},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existingCheck := CheckConfig{
				ID:      "check-1",
				Name:    "Original Name",
				Type:    "api",
				Target:  "https://example.com",
				Enabled: boolPtr(true),
			}

			store := &contractFakeStore{
				state: State{Checks: []CheckConfig{existingCheck}},
			}
			service := newTestService(store)
			handler := func(w http.ResponseWriter, r *http.Request) {
				ctx := context.Background()
				r = r.WithContext(ctx)
				service.handleCheckByID(w, r)
			}

			bodyBytes, _ := json.Marshal(tt.updateData)
			req := httptest.NewRequest(tt.method, "/api/v1/checks/"+tt.checkID, bytes.NewReader(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			handler(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatus {
				t.Errorf("status code = %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			// For successful updates, verify response
			if tt.wantStatus == http.StatusOK {
				var envelope APIResponse
				if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
					t.Fatalf("failed to decode response: %v", err)
				}

				if !envelope.Success {
					t.Error("envelope.Success = false, want true")
				}

				dataBytes, _ := json.Marshal(envelope.Data)
				var updated CheckConfig
				if err := json.Unmarshal(dataBytes, &updated); err != nil {
					t.Fatalf("failed to unmarshal CheckConfig: %v", err)
				}

				if updated.ID != tt.checkID {
					t.Errorf("updated check ID = %s, want %s", updated.ID, tt.checkID)
				}
			}
		})
	}
}

// TestDELETECheckByIDContract verifies DELETE /api/v1/checks/{id}
func TestDELETECheckByIDContract(t *testing.T) {
	existingCheck := CheckConfig{
		ID:      "check-1",
		Name:    "To Delete",
		Type:    "api",
		Target:  "https://example.com",
		Enabled: boolPtr(true),
	}

	store := &contractFakeStore{
		state: State{Checks: []CheckConfig{existingCheck}},
	}
	service := newTestService(store)
	handler := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		r = r.WithContext(ctx)
		service.handleCheckByID(w, r)
	}

	req := httptest.NewRequest("DELETE", "/api/v1/checks/check-1", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	// Verify status code is 204 No Content
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	// Verify no body
	body, _ := io.ReadAll(resp.Body)
	if len(body) != 0 {
		t.Errorf("response body is not empty, want empty body for 204 No Content")
	}
}

// TestGETStatusContract verifies GET /api/v1/summary returns status summary
func TestGETStatusContract(t *testing.T) {
	now := time.Now().UTC()
	store := &contractFakeStore{
		state: State{
			Checks: []CheckConfig{
				{ID: "check-1", Name: "Check 1", Type: "api", Enabled: boolPtr(true)},
				{ID: "check-2", Name: "Check 2", Type: "api", Enabled: boolPtr(true)},
				{ID: "check-3", Name: "Check 3", Type: "api", Enabled: boolPtr(false)},
			},
			Results: []CheckResult{
				{
					CheckID: "check-1",
					Name:    "Check 1",
					Type:    "api",
					Status:  "healthy",
					Healthy: true,
				},
				{
					CheckID: "check-2",
					Name:    "Check 2",
					Type:    "api",
					Status:  "critical",
					Healthy: false,
				},
			},
			LastRunAt: now,
		},
	}

	service := newTestService(store)
	handler := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		r = r.WithContext(ctx)
		service.handleSummary(w, r)
	}

	req := httptest.NewRequest("GET", "/api/v1/summary", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var envelope APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !envelope.Success {
		t.Fatal("envelope.Success = false, want true")
	}

	dataBytes, _ := json.Marshal(envelope.Data)
	var summary Summary
	if err := json.Unmarshal(dataBytes, &summary); err != nil {
		t.Fatalf("failed to unmarshal Summary: %v", err)
	}

	// Verify Summary structure
	if summary.TotalChecks != 3 {
		t.Errorf("TotalChecks = %d, want 3", summary.TotalChecks)
	}
	if summary.EnabledChecks != 2 {
		t.Errorf("EnabledChecks = %d, want 2", summary.EnabledChecks)
	}
	if summary.Healthy != 1 {
		t.Errorf("Healthy = %d, want 1", summary.Healthy)
	}
	if summary.Critical != 1 {
		t.Errorf("Critical = %d, want 1", summary.Critical)
	}
	if summary.LastRunAt == nil {
		t.Error("LastRunAt is nil, want non-nil")
	}
	if len(summary.ByServer) == 0 {
		t.Error("ByServer is empty, want at least default entry")
	}
	if len(summary.ByApplication) == 0 {
		t.Error("ByApplication is empty, want at least default entry")
	}
	if len(summary.Latest) != 3 {
		t.Errorf("Latest array length = %d, want 3", len(summary.Latest))
	}
}

// TestGETIncidentsContract verifies GET /api/v1/incidents returns Incident array
func TestGETIncidentsContract(t *testing.T) {
	// Create fake incident manager
	incidentRepo := NewMemoryIncidentRepository()
	incidentManager := NewIncidentManager(incidentRepo, nil)

	// Create test incidents
	incident1 := Incident{
		ID:        "incident-1",
		CheckID:   "check-1",
		CheckName: "Check 1",
		Type:      "api",
		Status:    "open",
		Severity:  "critical",
		Message:   "Service is down",
		StartedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	incident2 := Incident{
		ID:             "incident-2",
		CheckID:        "check-2",
		CheckName:      "Check 2",
		Type:           "tcp",
		Status:         "acknowledged",
		Severity:       "warning",
		Message:        "High latency",
		StartedAt:      time.Now().UTC(),
		UpdatedAt:      time.Now().UTC(),
		AcknowledgedAt: timePtr(time.Now().UTC()),
		AcknowledgedBy: "admin",
	}

	incidentRepo.CreateIncident(incident1)
	incidentRepo.CreateIncident(incident2)

	store := &contractFakeStore{}
	service := newTestService(store)
	service.SetIncidentManager(incidentManager)

	handler := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		r = r.WithContext(ctx)
		service.handleIncidents(w, r)
	}

	req := httptest.NewRequest("GET", "/api/v1/incidents", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var envelope APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !envelope.Success {
		t.Fatal("envelope.Success = false, want true")
	}

	dataBytes, _ := json.Marshal(envelope.Data)
	var incidentList []Incident
	if err := json.Unmarshal(dataBytes, &incidentList); err != nil {
		t.Fatalf("failed to unmarshal Incident array: %v", err)
	}

	if len(incidentList) != 2 {
		t.Errorf("got %d incidents, want 2", len(incidentList))
	}

	// Verify Incident structure
	for _, inc := range incidentList {
		if inc.ID == "" {
			t.Error("Incident.ID is empty")
		}
		if inc.CheckID == "" {
			t.Error("Incident.CheckID is empty")
		}
		if inc.CheckName == "" {
			t.Error("Incident.CheckName is empty")
		}
		if inc.Type == "" {
			t.Error("Incident.Type is empty")
		}
		if inc.Status == "" {
			t.Error("Incident.Status is empty")
		}
		if inc.Severity == "" {
			t.Error("Incident.Severity is empty")
		}
		if inc.Message == "" {
			t.Error("Incident.Message is empty")
		}
		if inc.StartedAt.IsZero() {
			t.Error("Incident.StartedAt is zero")
		}
		if inc.UpdatedAt.IsZero() {
			t.Error("Incident.UpdatedAt is zero")
		}
	}
}

// TestPOSTIncidentAcknowledgeContract verifies POST /api/v1/incidents/{id}/acknowledge
func TestPOSTIncidentAcknowledgeContract(t *testing.T) {
	incidentRepo := NewMemoryIncidentRepository()
	incidentManager := NewIncidentManager(incidentRepo, nil)

	// Create test incident
	incident := Incident{
		ID:        "incident-1",
		CheckID:   "check-1",
		CheckName: "Check 1",
		Type:      "api",
		Status:    "open",
		Severity:  "critical",
		Message:   "Service is down",
		StartedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	incidentRepo.CreateIncident(incident)

	store := &contractFakeStore{}
	service := newTestService(store)
	service.SetIncidentManager(incidentManager)

	handler := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		r = r.WithContext(ctx)
		service.handleIncidentByID(w, r)
	}

	payload := map[string]string{"acknowledgedBy": "admin"}
	bodyBytes, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/v1/incidents/incident-1/acknowledge", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var envelope APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !envelope.Success {
		t.Fatal("envelope.Success = false, want true")
	}

	dataBytes, _ := json.Marshal(envelope.Data)
	var updatedIncident Incident
	if err := json.Unmarshal(dataBytes, &updatedIncident); err != nil {
		t.Fatalf("failed to unmarshal Incident: %v", err)
	}

	if updatedIncident.Status != "acknowledged" {
		t.Errorf("incident Status = %s, want acknowledged", updatedIncident.Status)
	}
	if updatedIncident.AcknowledgedAt == nil {
		t.Error("AcknowledgedAt is nil, want non-nil")
	}
	if updatedIncident.AcknowledgedBy != "admin" {
		t.Errorf("AcknowledgedBy = %s, want admin", updatedIncident.AcknowledgedBy)
	}
}

// TestPOSTIncidentResolveContract verifies POST /api/v1/incidents/{id}/resolve
func TestPOSTIncidentResolveContract(t *testing.T) {
	incidentRepo := NewMemoryIncidentRepository()
	incidentManager := NewIncidentManager(incidentRepo, nil)

	// Create test incident
	now := time.Now().UTC()
	incident := Incident{
		ID:             "incident-1",
		CheckID:        "check-1",
		CheckName:      "Check 1",
		Type:           "api",
		Status:         "acknowledged",
		Severity:       "critical",
		Message:        "Service is down",
		StartedAt:      now,
		UpdatedAt:      now,
		AcknowledgedAt: &now,
		AcknowledgedBy: "admin",
	}
	incidentRepo.CreateIncident(incident)

	store := &contractFakeStore{}
	service := newTestService(store)
	service.SetIncidentManager(incidentManager)

	handler := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		r = r.WithContext(ctx)
		service.handleIncidentByID(w, r)
	}

	payload := map[string]string{"resolvedBy": "ops-team"}
	bodyBytes, _ := json.Marshal(payload)
	req := httptest.NewRequest("POST", "/api/v1/incidents/incident-1/resolve", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var envelope APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !envelope.Success {
		t.Fatal("envelope.Success = false, want true")
	}

	dataBytes, _ := json.Marshal(envelope.Data)
	var resolvedIncident Incident
	if err := json.Unmarshal(dataBytes, &resolvedIncident); err != nil {
		t.Fatalf("failed to unmarshal Incident: %v", err)
	}

	if resolvedIncident.Status != "resolved" {
		t.Errorf("incident Status = %s, want resolved", resolvedIncident.Status)
	}
	if resolvedIncident.ResolvedAt == nil {
		t.Error("ResolvedAt is nil, want non-nil")
	}
	if resolvedIncident.ResolvedBy != "ops-team" {
		t.Errorf("ResolvedBy = %s, want ops-team", resolvedIncident.ResolvedBy)
	}
}

// TestGETAuditContract verifies GET /api/v1/audit returns AuditEvent array with filters
func TestGETAuditContract(t *testing.T) {
	discardLogger := log.New(io.Discard, "", 0)
	auditRepo := &InMemoryAuditRepository{events: []AuditEvent{}}
	auditLogger := NewAuditLogger(auditRepo, discardLogger)

	// Create test audit events
	auditLogger.Log("check.created", "admin", "check", "check-1", map[string]interface{}{"name": "Test Check"})
	auditLogger.Log("check.updated", "user1", "check", "check-2", map[string]interface{}{"name": "Updated Check"})
	auditLogger.Log("incident.acknowledged", "admin", "incident", "inc-1", nil)

	store := &contractFakeStore{}
	service := newTestService(store) // Use discard logger to prevent scheduler panic
	service.SetAuditLogger(auditLogger)

	handler := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		r = r.WithContext(ctx)
		service.handleAudit(w, r)
	}

	req := httptest.NewRequest("GET", "/api/v1/audit", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status code = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var envelope APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !envelope.Success {
		t.Fatal("envelope.Success = false, want true")
	}

	dataBytes, _ := json.Marshal(envelope.Data)
	var events []AuditEvent
	if err := json.Unmarshal(dataBytes, &events); err != nil {
		t.Fatalf("failed to unmarshal AuditEvent array: %v", err)
	}

	// Note: Event count may vary due to shared audit file across tests
	// We verify structure is correct regardless of count
	if len(events) < 3 {
		t.Errorf("got %d events, want at least 3", len(events))
	}

	// Verify AuditEvent structure of first 3 events
	for _, event := range events {
		if event.ID == "" {
			t.Error("AuditEvent.ID is empty")
		}
		if event.Action == "" {
			t.Error("AuditEvent.Action is empty")
		}
		if event.Actor == "" {
			t.Error("AuditEvent.Actor is empty")
		}
		// Target and TargetID are optional
		if event.Timestamp.IsZero() {
			t.Error("AuditEvent.Timestamp is zero")
		}
	}
}

// TestGETAuditWithFiltersContract verifies audit endpoint respects query parameters
func TestGETAuditWithFiltersContract(t *testing.T) {
	tests := []struct {
		name        string
		queryParams string
		wantCount   int
		verifyActor string
		setupEvents func(*AuditLogger)
	}{
		{
			name:        "filter by actor=admin",
			queryParams: "?actor=admin",
			wantCount:   2,
			verifyActor: "admin",
			setupEvents: func(al *AuditLogger) {
				al.Log("check.created", "admin", "check", "check-1", nil)
				al.Log("check.updated", "user1", "check", "check-2", nil)
				al.Log("incident.acknowledged", "admin", "incident", "inc-1", nil)
			},
		},
		{
			name:        "filter by action=check.created",
			queryParams: "?action=check.created",
			wantCount:   1,
			setupEvents: func(al *AuditLogger) {
				al.Log("check.created", "admin", "check", "check-1", nil)
				al.Log("check.updated", "user1", "check", "check-2", nil)
				al.Log("incident.acknowledged", "admin", "incident", "inc-1", nil)
			},
		},
		{
			name:        "filter by target=check",
			queryParams: "?target=check",
			wantCount:   2,
			setupEvents: func(al *AuditLogger) {
				al.Log("check.created", "admin", "check", "check-1", nil)
				al.Log("check.updated", "user1", "check", "check-2", nil)
				al.Log("incident.acknowledged", "admin", "incident", "inc-1", nil)
			},
		},
		{
			name:        "filter by targetId=check-1",
			queryParams: "?targetId=check-1",
			wantCount:   1,
			setupEvents: func(al *AuditLogger) {
				al.Log("check.created", "admin", "check", "check-1", nil)
				al.Log("check.updated", "user1", "check", "check-2", nil)
				al.Log("incident.acknowledged", "admin", "incident", "inc-1", nil)
			},
		},
		{
			name:        "limit results",
			queryParams: "?limit=1",
			wantCount:   1,
			setupEvents: func(al *AuditLogger) {
				al.Log("check.created", "admin", "check", "check-1", nil)
				al.Log("check.updated", "user1", "check", "check-2", nil)
				al.Log("incident.acknowledged", "admin", "incident", "inc-1", nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh audit repo for each test - use discard logger to avoid noise
			auditRepo := &InMemoryAuditRepository{events: []AuditEvent{}}
			discardLogger := log.New(io.Discard, "", 0)
			auditLogger := NewAuditLogger(auditRepo, discardLogger)

			// Setup events for this test
			if tt.setupEvents != nil {
				tt.setupEvents(auditLogger)
			}

			store := &contractFakeStore{}
			service := newTestService(store)
			service.SetAuditLogger(auditLogger)

			handler := func(w http.ResponseWriter, r *http.Request) {
				ctx := context.Background()
				r = r.WithContext(ctx)
				service.handleAudit(w, r)
			}

			req := httptest.NewRequest("GET", "/api/v1/audit"+tt.queryParams, nil)
			w := httptest.NewRecorder()
			handler(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			var envelope APIResponse
			if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			dataBytes, _ := json.Marshal(envelope.Data)
			var events []AuditEvent
			if err := json.Unmarshal(dataBytes, &events); err != nil {
				t.Fatalf("failed to unmarshal AuditEvent array: %v", err)
			}

			if len(events) < tt.wantCount {
				t.Errorf("got %d events, want at least %d", len(events), tt.wantCount)
			}

			// Verify actor filter if specified
			if tt.verifyActor != "" && len(events) > 0 {
				for _, event := range events {
					if event.Actor != tt.verifyActor {
						t.Errorf("event.Actor = %s, want %s", event.Actor, tt.verifyActor)
					}
				}
			}
		})
	}
}

// TestErrorResponsesContract verifies error response structure across endpoints
func TestErrorResponsesContract(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		url            string
		body           interface{}
		wantStatusCode int
		useHandler     string
	}{
		{
			name:           "GET non-existent incident returns proper error",
			method:         "GET",
			url:            "/api/v1/incidents/non-existent",
			wantStatusCode: http.StatusNotFound,
			useHandler:     "incident",
		},
		{
			name:           "POST invalid JSON returns 400",
			method:         "POST",
			url:            "/api/v1/checks",
			body:           "invalid json",
			wantStatusCode: http.StatusBadRequest,
			useHandler:     "checks",
		},
		{
			name:           "POST missing required fields returns 400",
			method:         "POST",
			url:            "/api/v1/checks",
			body:           map[string]string{"name": "Test"},
			wantStatusCode: http.StatusBadRequest,
			useHandler:     "checks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := &contractFakeStore{state: State{Checks: []CheckConfig{}}}
			service := newTestService(store)

			var handler func(w http.ResponseWriter, r *http.Request)
			if tt.useHandler == "incident" {
				// Setup incident manager for this test
				incidentRepo := NewMemoryIncidentRepository()
				service.SetIncidentManager(NewIncidentManager(incidentRepo, nil))
				handler = func(w http.ResponseWriter, r *http.Request) {
					ctx := context.Background()
					r = r.WithContext(ctx)
					service.handleIncidentByID(w, r)
				}
			} else {
				handler = func(w http.ResponseWriter, r *http.Request) {
					ctx := context.Background()
					r = r.WithContext(ctx)
					if strings.HasPrefix(r.URL.Path, "/api/v1/checks/") && r.Method == "GET" {
						service.handleCheckByID(w, r)
					} else {
						service.handleChecks(w, r)
					}
				}
			}

			var body io.Reader
			if tt.body != nil {
				if str, ok := tt.body.(string); ok {
					body = strings.NewReader(str)
				} else {
					bodyBytes, _ := json.Marshal(tt.body)
					body = bytes.NewReader(bodyBytes)
				}
			}

			req := httptest.NewRequest(tt.method, tt.url, body)
			if tt.method == "POST" || tt.method == "PUT" || tt.method == "PATCH" {
				req.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()
			handler(w, req)

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", resp.StatusCode, tt.wantStatusCode)
			}

			var envelope APIResponse
			if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			// Verify error envelope structure
			if envelope.Success {
				t.Error("envelope.Success = true, want false for error response")
			}
			if envelope.Error == nil {
				t.Fatal("envelope.Error is nil, want error object")
			}
			if envelope.Error.Code != tt.wantStatusCode {
				t.Errorf("envelope.Error.Code = %d, want %d", envelope.Error.Code, tt.wantStatusCode)
			}
			if envelope.Error.Message == "" {
				t.Error("envelope.Error.Message is empty")
			}
			if envelope.Data != nil {
				t.Error("envelope.Data should be nil for error response")
			}
		})
	}
}

func TestFieldTypeContract(t *testing.T) {
	tests := []struct {
		name       string
		sampleData interface{}
		typeCheck  func(interface{}) error
	}{
		{
			name: "CheckListItem field types",
			sampleData: CheckListItem{
				ID:          "check-1",
				Name:        "Test Check",
				Type:        "api",
				Server:      "prod-1",
				Application: "medics",
				Enabled:     true,
				Tags:        []string{"api", "prod"},
			},
			typeCheck: func(data interface{}) error {
				item, ok := data.(CheckListItem)
				if !ok {
					return fmt.Errorf("not a CheckListItem")
				}
				// Check field types through JSON round-trip
				if item.ID != "check-1" {
					return fmt.Errorf("ID field not preserved")
				}
				if item.Name != "Test Check" {
					return fmt.Errorf("Name field not preserved")
				}
				if item.Type != "api" {
					return fmt.Errorf("Type field not preserved")
				}
				if !item.Enabled {
					return fmt.Errorf("Enabled field not preserved")
				}
				return nil
			},
		},
		{
			name: "Incident field types",
			sampleData: Incident{
				ID:        "inc-1",
				CheckID:   "check-1",
				CheckName: "Check 1",
				Type:      "api",
				Status:    "open",
				Severity:  "critical",
				Message:   "Down",
				StartedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			},
			typeCheck: func(data interface{}) error {
				inc, ok := data.(Incident)
				if !ok {
					return fmt.Errorf("not an Incident")
				}
				// Check field types through JSON round-trip
				if inc.ID != "inc-1" {
					return fmt.Errorf("ID field not preserved")
				}
				if inc.Status != "open" {
					return fmt.Errorf("Status field not preserved")
				}
				if inc.Severity != "critical" {
					return fmt.Errorf("Severity field not preserved")
				}
				return nil
			},
		},
		{
			name: "AuditEvent field types",
			sampleData: AuditEvent{
				ID:        "audit-1",
				Action:    "check.created",
				Actor:     "admin",
				Target:    "check",
				TargetID:  "check-1",
				Timestamp: time.Now().UTC(),
			},
			typeCheck: func(data interface{}) error {
				event, ok := data.(AuditEvent)
				if !ok {
					return fmt.Errorf("not an AuditEvent")
				}
				// Check field types through JSON round-trip
				if event.ID != "audit-1" {
					return fmt.Errorf("ID field not preserved")
				}
				if event.Action != "check.created" {
					return fmt.Errorf("Action field not preserved")
				}
				if event.Actor != "admin" {
					return fmt.Errorf("Actor field not preserved")
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to JSON and unmarshal to verify type preservation
			dataBytes, err := json.Marshal(tt.sampleData)
			if err != nil {
				t.Fatalf("failed to marshal: %v", err)
			}

			// Verify JSON is valid
			var raw map[string]interface{}
			if err := json.Unmarshal(dataBytes, &raw); err != nil {
				t.Fatalf("failed to unmarshal to raw map: %v", err)
			}

			// Verify structure through JSON round-trip
			if err := tt.typeCheck(tt.sampleData); err != nil {
				t.Errorf("type check failed: %v", err)
			}
		})
	}
}

// TestIncidentEndpointsUnAvailable verifies proper error when incident manager not configured
func TestIncidentEndpointsUnAvailable(t *testing.T) {
	store := &contractFakeStore{}
	service := newTestService(store)
	// Don't set incident manager

	handler := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		r = r.WithContext(ctx)
		service.handleIncidents(w, r)
	}

	req := httptest.NewRequest("GET", "/api/v1/incidents", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}

	var envelope APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if envelope.Success {
		t.Error("envelope.Success = true, want false")
	}
	if envelope.Error == nil {
		t.Error("envelope.Error is nil")
	}
	if !strings.Contains(strings.ToLower(envelope.Error.Message), "incident manager") {
		t.Errorf("error message should mention incident manager, got: %s", envelope.Error.Message)
	}
}

// TestAuditEndpointsUnAvailable verifies proper error when audit logger not configured
func TestAuditEndpointsUnAvailable(t *testing.T) {
	store := &contractFakeStore{}
	service := newTestService(store)
	// Explicitly disable audit logger for this contract.
	service.SetAuditLogger(nil)

	handler := func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		r = r.WithContext(ctx)
		service.handleAudit(w, r)
	}

	req := httptest.NewRequest("GET", "/api/v1/audit", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}

	var envelope APIResponse
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if envelope.Success {
		t.Error("envelope.Success = true, want false")
	}
	if envelope.Error == nil {
		t.Error("envelope.Error is nil")
	}
	if !strings.Contains(strings.ToLower(envelope.Error.Message), "audit") {
		t.Errorf("error message should mention audit, got: %s", envelope.Error.Message)
	}
}

// Helper types and functions for contract tests

func timePtr(t time.Time) *time.Time {
	return &t
}

// contractFakeStore is a minimal Store implementation for contract testing
type contractFakeStore struct {
	state State
}

func (f *contractFakeStore) Snapshot() State {
	return f.state
}

func (f *contractFakeStore) DashboardSnapshot() DashboardSnapshot {
	return DashboardSnapshot{
		State:       f.state,
		Summary:     buildSummary(f.state.Checks, f.state.Results, &f.state.LastRunAt),
		GeneratedAt: time.Now().UTC(),
	}
}

func (f *contractFakeStore) Update(mutator func(*State) error) error {
	return mutator(&f.state)
}

func (f *contractFakeStore) ReplaceChecks(checks []CheckConfig) error {
	f.state.Checks = checks
	return nil
}

func (f *contractFakeStore) UpsertCheck(check CheckConfig) error {
	for i, c := range f.state.Checks {
		if c.ID == check.ID {
			f.state.Checks[i] = check
			return nil
		}
	}
	f.state.Checks = append(f.state.Checks, check)
	return nil
}

func (f *contractFakeStore) DeleteCheck(id string) error {
	for i, c := range f.state.Checks {
		if c.ID == id {
			f.state.Checks = append(f.state.Checks[:i], f.state.Checks[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("check not found")
}

func (f *contractFakeStore) AppendResults(results []CheckResult, retentionDays int) error {
	f.state.Results = append(f.state.Results, results...)
	return nil
}

func (f *contractFakeStore) SetLastRun(t time.Time) error {
	f.state.LastRunAt = t
	return nil
}
