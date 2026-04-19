package monitoring

import (
	"fmt"
	"log"
	"time"
)

// IncidentRepository defines the interface for incident persistence
type IncidentRepository interface {
	CreateIncident(incident Incident) error
	UpdateIncident(id string, mutator func(*Incident) error) error
	GetIncident(id string) (Incident, error)
	ListIncidents() ([]Incident, error)
	FindOpenIncident(checkID string) (Incident, error)
}

// IncidentCreatedCallback is called when a new incident is created.
type IncidentCreatedCallback func(incident Incident)

// IncidentManager handles incident lifecycle
type IncidentManager struct {
	repo              IncidentRepository
	logger            *log.Logger
	onIncidentCreated IncidentCreatedCallback
}

// NewIncidentManager creates a new incident manager
func NewIncidentManager(repo IncidentRepository, logger *log.Logger) *IncidentManager {
	if logger == nil {
		logger = log.Default()
	}
	return &IncidentManager{
		repo:   repo,
		logger: logger,
	}
}

// ProcessAlert processes an alert and creates or updates incidents
func (im *IncidentManager) ProcessAlert(checkID, checkName, checkType, severity, message string, metadata map[string]string) error {
	// Check if there's already an open incident for this check
	incident, err := im.repo.FindOpenIncident(checkID)
	if err != nil {
		im.logger.Printf("Warning: failed to find open incident: %v", err)
		// Continue to create new incident
	}

	if incident.ID != "" {
		// Update existing incident
		return im.updateIncident(incident, severity, message, metadata)
	}

	// Create new incident
	return im.createIncident(checkID, checkName, checkType, severity, message, metadata)
}

// createIncident creates a new incident from an alert
func (im *IncidentManager) createIncident(checkID, checkName, checkType, severity, message string, metadata map[string]string) error {
	now := time.Now().UTC()
	incident := Incident{
		ID:        generateIncidentID(checkID, now),
		CheckID:   checkID,
		CheckName: checkName,
		Type:      checkType,
		Status:    "open",
		Severity:  severity,
		Message:   message,
		StartedAt: now,
		UpdatedAt: now,
		Metadata:  metadata,
	}

	if err := im.repo.CreateIncident(incident); err != nil {
		return fmt.Errorf("create incident: %w", err)
	}

	im.logger.Printf("Incident created: %s for check %s (%s)", incident.ID, checkID, severity)

	// Notify callback (e.g. AI analysis enqueue)
	if im.onIncidentCreated != nil {
		im.onIncidentCreated(incident)
	}

	return nil
}

// updateIncident updates an existing incident with new alert information
func (im *IncidentManager) updateIncident(incident Incident, severity, message string, metadata map[string]string) error {
	return im.repo.UpdateIncident(incident.ID, func(inc *Incident) error {
		// Update severity if it has increased
		if severityHasIncreased(inc.Severity, severity) {
			inc.Severity = severity
		}

		// Update message and metadata
		inc.Message = message
		if inc.Metadata == nil {
			inc.Metadata = make(map[string]string)
		}
		for k, v := range metadata {
			inc.Metadata[k] = v
		}

		inc.UpdatedAt = time.Now().UTC()
		return nil
	})
}

// AcknowledgeIncident acknowledges an incident
func (im *IncidentManager) AcknowledgeIncident(id, acknowledgedBy string) error {
	return im.repo.UpdateIncident(id, func(inc *Incident) error {
		if inc.Status == "resolved" {
			return fmt.Errorf("cannot acknowledge resolved incident")
		}

		now := time.Now().UTC()
		inc.Status = "acknowledged"
		inc.AcknowledgedAt = &now
		inc.AcknowledgedBy = acknowledgedBy
		inc.UpdatedAt = now

		im.logger.Printf("Incident acknowledged: %s by %s", id, acknowledgedBy)
		return nil
	})
}

// ResolveIncident resolves an incident
func (im *IncidentManager) ResolveIncident(id, resolvedBy string) error {
	return im.repo.UpdateIncident(id, func(inc *Incident) error {
		if inc.Status == "resolved" {
			return fmt.Errorf("incident already resolved")
		}

		now := time.Now().UTC()
		inc.Status = "resolved"
		inc.ResolvedAt = &now
		inc.ResolvedBy = resolvedBy
		inc.UpdatedAt = now

		im.logger.Printf("Incident resolved: %s by %s", id, resolvedBy)
		return nil
	})
}

// SetOnIncidentCreated sets a callback for new incidents (e.g. AI analysis enqueue).
func (im *IncidentManager) SetOnIncidentCreated(cb IncidentCreatedCallback) {
	im.onIncidentCreated = cb
}

// AutoResolveOnRecovery automatically resolves an incident when a check recovers
func (im *IncidentManager) AutoResolveOnRecovery(checkID string) error {
	incident, err := im.repo.FindOpenIncident(checkID)
	if err != nil {
		return fmt.Errorf("find open incident: %w", err)
	}

	if incident.ID == "" {
		// No open incident, nothing to do
		return nil
	}

	return im.ResolveIncident(incident.ID, "system")
}

// severityHasIncreased checks if the new severity is higher than the old one
func severityHasIncreased(old, new string) bool {
	severities := map[string]int{
		"warning":  1,
		"critical": 2,
	}

	oldLevel, ok := severities[old]
	if !ok {
		return true
	}

	newLevel, ok := severities[new]
	if !ok {
		return false
	}

	return newLevel > oldLevel
}

// generateIncidentID generates a unique incident ID
func generateIncidentID(checkID string, t time.Time) string {
	return fmt.Sprintf("%s-%d", checkID, t.Unix())
}
