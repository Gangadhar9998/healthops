package monitoring

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
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
