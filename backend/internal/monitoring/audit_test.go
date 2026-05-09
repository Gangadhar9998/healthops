package monitoring

import (
	"net/http"
	"testing"
	"time"
)

// InMemoryAuditRepository is a test implementation of AuditRepository
type InMemoryAuditRepository struct {
	events []AuditEvent
}

func (r *InMemoryAuditRepository) InsertEvent(event AuditEvent) error {
	r.events = append(r.events, event)
	return nil
}

func (r *InMemoryAuditRepository) ListEvents(filter AuditFilter) ([]AuditEvent, error) {
	var results []AuditEvent

	for _, event := range r.events {
		if filter.Action != "" && event.Action != filter.Action {
			continue
		}
		if filter.Actor != "" && event.Actor != filter.Actor {
			continue
		}
		if filter.Target != "" && event.Target != filter.Target {
			continue
		}
		if filter.TargetID != "" && event.TargetID != filter.TargetID {
			continue
		}
		if !filter.StartTime.IsZero() && event.Timestamp.Before(filter.StartTime) {
			continue
		}
		if !filter.EndTime.IsZero() && event.Timestamp.After(filter.EndTime) {
			continue
		}
		results = append(results, event)
	}

	// Sort by timestamp descending (simple implementation)
	for i := 0; i < len(results)-1; i++ {
		for j := 0; j < len(results)-i-1; j++ {
			if results[j].Timestamp.Before(results[j+1].Timestamp) {
				results[j], results[j+1] = results[j+1], results[j]
			}
		}
	}

	// Apply pagination
	if filter.Offset > 0 {
		if filter.Offset >= len(results) {
			return []AuditEvent{}, nil
		}
		results = results[filter.Offset:]
	}
	if filter.Limit > 0 && filter.Limit < len(results) {
		results = results[:filter.Limit]
	}

	return results, nil
}

func TestAuditLogger_Log(t *testing.T) {
	repo := &InMemoryAuditRepository{events: []AuditEvent{}}
	logger := NewAuditLogger(repo, nil)

	tests := []struct {
		name      string
		action    string
		actor     string
		target    string
		targetID  string
		details   map[string]interface{}
		wantError bool
	}{
		{
			name:     "log check created",
			action:   "check.created",
			actor:    "admin",
			target:   "check",
			targetID: "check-001",
			details:  map[string]interface{}{"name": "API Health Check"},
		},
		{
			name:     "log check updated",
			action:   "check.updated",
			actor:    "user1",
			target:   "check",
			targetID: "check-002",
			details:  map[string]interface{}{"changes": "interval"},
		},
		{
			name:     "log check deleted",
			action:   "check.deleted",
			actor:    "admin",
			target:   "check",
			targetID: "check-003",
			details:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := logger.Log(tt.action, tt.actor, tt.target, tt.targetID, tt.details)
			if (err != nil) != tt.wantError {
				t.Errorf("Log() error = %v, wantError %v", err, tt.wantError)
				return
			}

			// Verify event was logged
			if len(repo.events) == 0 {
				t.Error("No events were logged")
				return
			}

			event := repo.events[len(repo.events)-1]
			if event.Action != tt.action {
				t.Errorf("Action = %v, want %v", event.Action, tt.action)
			}
			if event.Actor != tt.actor {
				t.Errorf("Actor = %v, want %v", event.Actor, tt.actor)
			}
			if event.Target != tt.target {
				t.Errorf("Target = %v, want %v", event.Target, tt.target)
			}
			if event.TargetID != tt.targetID {
				t.Errorf("TargetID = %v, want %v", event.TargetID, tt.targetID)
			}
			if event.ID == "" {
				t.Error("ID should not be empty")
			}
			if event.Timestamp.IsZero() {
				t.Error("Timestamp should not be zero")
			}
		})
	}
}

func TestAuditLogger_GetAuditEvents(t *testing.T) {
	repo := &InMemoryAuditRepository{events: []AuditEvent{}}
	logger := NewAuditLogger(repo, nil)

	// Create test events
	now := time.Now().UTC()
	testEvents := []AuditEvent{
		{
			ID:        "audit-001",
			Action:    "check.created",
			Actor:     "admin",
			Target:    "check",
			TargetID:  "check-001",
			Timestamp: now.Add(-2 * time.Hour),
		},
		{
			ID:        "audit-002",
			Action:    "check.updated",
			Actor:     "user1",
			Target:    "check",
			TargetID:  "check-002",
			Timestamp: now.Add(-1 * time.Hour),
		},
		{
			ID:        "audit-003",
			Action:    "check.created",
			Actor:     "admin",
			Target:    "check",
			TargetID:  "check-003",
			Timestamp: now,
		},
	}

	for _, event := range testEvents {
		_ = repo.InsertEvent(event)
	}

	tests := []struct {
		name    string
		filter  AuditFilter
		wantLen int
	}{
		{
			name:    "get all events",
			filter:  AuditFilter{},
			wantLen: 3,
		},
		{
			name:    "filter by action",
			filter:  AuditFilter{Action: "check.created"},
			wantLen: 2,
		},
		{
			name:    "filter by actor",
			filter:  AuditFilter{Actor: "admin"},
			wantLen: 2,
		},
		{
			name:    "filter by target ID",
			filter:  AuditFilter{TargetID: "check-002"},
			wantLen: 1,
		},
		{
			name:    "filter by target",
			filter:  AuditFilter{Target: "check"},
			wantLen: 3,
		},
		{
			name:    "filter with limit",
			filter:  AuditFilter{Limit: 2},
			wantLen: 2,
		},
		{
			name:    "filter with offset",
			filter:  AuditFilter{Offset: 1},
			wantLen: 2,
		},
		{
			name:    "filter by start time",
			filter:  AuditFilter{StartTime: now.Add(-90 * time.Minute)},
			wantLen: 2,
		},
		{
			name:    "filter by end time",
			filter:  AuditFilter{EndTime: now.Add(-90 * time.Minute)},
			wantLen: 1,
		},
		{
			name:    "combined filters",
			filter:  AuditFilter{Action: "check.created", Actor: "admin"},
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := logger.GetAuditEvents(tt.filter)
			if err != nil {
				t.Errorf("GetAuditEvents() error = %v", err)
				return
			}
			if len(events) != tt.wantLen {
				t.Errorf("GetAuditEvents() len = %v, want %v", len(events), tt.wantLen)
			}
		})
	}
}

func TestGenerateAuditID(t *testing.T) {
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateAuditID()
		if id == "" {
			t.Error("generateAuditID() returned empty string")
		}
		if ids[id] {
			t.Errorf("generateAuditID() returned duplicate ID: %s", id)
		}
		ids[id] = true
	}
}

func TestExtractActorFromRequest(t *testing.T) {
	cfg := &Config{}

	// Test with auth disabled
	cfg.Auth.Enabled = false
	req := &http.Request{}
	actor := ExtractActorFromRequest(req, cfg)
	if actor != "system" {
		t.Errorf("ExtractActorFromRequest() with auth disabled = %v, want 'system'", actor)
	}

	// Test with auth enabled and Basic Auth
	cfg.Auth.Enabled = true
	req = &http.Request{
		Header: http.Header{},
	}
	req.SetBasicAuth("testuser", "password")
	actor = ExtractActorFromRequest(req, cfg)
	if actor != "testuser" {
		t.Errorf("ExtractActorFromRequest() with Basic Auth = %v, want 'testuser'", actor)
	}

	// Test with X-User header
	req = &http.Request{
		Header: http.Header{},
	}
	req.Header.Set("X-User", "api-user")
	actor = ExtractActorFromRequest(req, cfg)
	if actor != "api-user" {
		t.Errorf("ExtractActorFromRequest() with X-User header = %v, want 'api-user'", actor)
	}

	// Test with no auth info
	req = &http.Request{
		Header: http.Header{},
	}
	actor = ExtractActorFromRequest(req, cfg)
	if actor != "unknown" {
		t.Errorf("ExtractActorFromRequest() with no auth = %v, want 'unknown'", actor)
	}
}
