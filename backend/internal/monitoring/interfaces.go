package monitoring

import "time"

// NotificationOutboxRepository defines generic notification queue operations.
type NotificationOutboxRepository interface {
	Enqueue(evt NotificationEvent) error
	ListPending(limit int) ([]NotificationEvent, error)
	MarkSent(id string) error
	MarkFailed(id string, reason string) error
	PruneBefore(cutoff time.Time) error
	AllNotifications() []NotificationEvent
}

// AIQueueRepository defines generic AI analysis queue operations.
type AIQueueRepository interface {
	Enqueue(incidentID string, promptVersion string) error
	ClaimPending(limit int) ([]AIQueueItem, error)
	Complete(incidentID string, result AIAnalysisResult) error
	Fail(incidentID string, reason string) error
	PruneBefore(cutoff time.Time) error
	ListPendingItems(limit int) ([]AIQueueItem, error)
	AllItems() []AIQueueItem
}

// MySQLMetricsRepository defines persistence for MySQL samples and deltas.
type MySQLMetricsRepository interface {
	AppendSample(sample MySQLSample) (string, error)
	ComputeAndAppendDelta(sampleID string) (MySQLDelta, error)
	LatestSample(checkID string) (MySQLSample, error)
	RecentSamples(checkID string, limit int) ([]MySQLSample, error)
	RecentDeltas(checkID string, limit int) ([]MySQLDelta, error)
}
