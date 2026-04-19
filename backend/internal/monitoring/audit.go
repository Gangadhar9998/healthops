package monitoring

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AuditEvent represents a single audit log entry
type AuditEvent struct {
	ID        string                 `json:"id" bson:"_id"`
	Action    string                 `json:"action" bson:"action"`
	Actor     string                 `json:"actor" bson:"actor"`
	Target    string                 `json:"target,omitempty" bson:"target,omitempty"`
	TargetID  string                 `json:"targetId,omitempty" bson:"targetId,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty" bson:"details,omitempty"`
	Timestamp time.Time              `json:"timestamp" bson:"timestamp"`
}

// AuditFilter represents filters for querying audit events
type AuditFilter struct {
	Action    string    `json:"action,omitempty"`
	Actor     string    `json:"actor,omitempty"`
	Target    string    `json:"target,omitempty"`
	TargetID  string    `json:"targetId,omitempty"`
	StartTime time.Time `json:"startTime,omitempty"`
	EndTime   time.Time `json:"endTime,omitempty"`
	Limit     int       `json:"limit,omitempty"`
	Offset    int       `json:"offset,omitempty"`
}

// AuditRepository defines the interface for audit persistence
type AuditRepository interface {
	InsertEvent(event AuditEvent) error
	ListEvents(filter AuditFilter) ([]AuditEvent, error)
}

// FileAuditRepository implements audit logging to a file
type FileAuditRepository struct {
	mu     sync.RWMutex
	path   string
	events []AuditEvent
}

var _ AuditRepository = (*FileAuditRepository)(nil)

// NewFileAuditRepository creates a new file-based audit repository
func NewFileAuditRepository(path string) (*FileAuditRepository, error) {
	if path == "" {
		path = "data/audit.json"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create audit dir: %w", err)
	}

	repo := &FileAuditRepository{path: path}
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &repo.events); err != nil {
			return nil, fmt.Errorf("parse audit log: %w", err)
		}
	}

	return repo, nil
}

func (r *FileAuditRepository) InsertEvent(event AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.events = append(r.events, event)
	return r.flushLocked()
}

func (r *FileAuditRepository) ListEvents(filter AuditFilter) ([]AuditEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []AuditEvent

	// Apply filters
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

	// Sort by timestamp descending
	sortEventsDescending(results)

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

func (r *FileAuditRepository) flushLocked() error {
	encoded, err := json.MarshalIndent(r.events, "", "  ")
	if err != nil {
		return fmt.Errorf("encode audit log: %w", err)
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o644); err != nil {
		return fmt.Errorf("write temp audit log: %w", err)
	}
	if err := os.Rename(tmp, r.path); err != nil {
		return fmt.Errorf("replace audit log: %w", err)
	}
	return nil
}

func sortEventsDescending(events []AuditEvent) {
	// Simple bubble sort for descending timestamp
	n := len(events)
	for i := 0; i < n-1; i++ {
		for j := 0; j < n-i-1; j++ {
			if events[j].Timestamp.Before(events[j+1].Timestamp) {
				events[j], events[j+1] = events[j+1], events[j]
			}
		}
	}
}

// AuditLogger provides audit logging functionality
type AuditLogger struct {
	repo   AuditRepository
	logger *log.Logger
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(repo AuditRepository, logger *log.Logger) *AuditLogger {
	if logger == nil {
		logger = log.New(os.Stdout, "[AUDIT] ", log.LstdFlags)
	}
	return &AuditLogger{
		repo:   repo,
		logger: logger,
	}
}

// Log records an audit event
func (a *AuditLogger) Log(action, actor, target, targetID string, details map[string]interface{}) error {
	event := AuditEvent{
		ID:        generateAuditID(),
		Action:    action,
		Actor:     actor,
		Target:    target,
		TargetID:  targetID,
		Details:   details,
		Timestamp: time.Now().UTC(),
	}

	if err := a.repo.InsertEvent(event); err != nil {
		a.logger.Printf("Failed to write audit log: %v", err)
		return err
	}

	a.logger.Printf("%s: %s %s %s/%s", event.ID, action, actor, target, targetID)
	return nil
}

// GetAuditEvents retrieves audit events with optional filtering
func (a *AuditLogger) GetAuditEvents(filter AuditFilter) ([]AuditEvent, error) {
	return a.repo.ListEvents(filter)
}

// generateAuditID generates a unique audit ID
func generateAuditID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if crypto rand fails
		return fmt.Sprintf("audit-%d", time.Now().UnixNano())
	}
	return "audit-" + hex.EncodeToString(b)
}

// ExtractActorFromRequest extracts the actor from the request
// Returns "system" if authentication is disabled, username otherwise
func ExtractActorFromRequest(r *http.Request, cfg *Config) string {
	if !cfg.Auth.Enabled {
		return "system"
	}

	// Extract username from Basic Auth
	username, _, ok := r.BasicAuth()
	if ok && username != "" {
		return username
	}

	// Check for X-User header (for API keys or tokens)
	if username := r.Header.Get("X-User"); username != "" {
		return username
	}

	return "unknown"
}
