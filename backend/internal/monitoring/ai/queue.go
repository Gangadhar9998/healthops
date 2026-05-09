package ai

import (
	"time"

	"medics-health-check/backend/internal/monitoring"
)

// AIQueueRepository defines generic AI analysis queue operations.
// Reusable across all check types, not MySQL-specific.
type AIQueueRepository interface {
	Enqueue(incidentID string, promptVersion string) error
	ClaimPending(limit int) ([]monitoring.AIQueueItem, error)
	Complete(incidentID string, result monitoring.AIAnalysisResult) error
	Fail(incidentID string, reason string) error
	PruneBefore(cutoff time.Time) error
	GetResults(incidentID string) []monitoring.AIAnalysisResult
	AllResults(limit int) []monitoring.AIAnalysisResult
	ListPendingItems(limit int) ([]monitoring.AIQueueItem, error)
	AllItems() []monitoring.AIQueueItem
}
