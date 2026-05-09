package mysql

import (
	"fmt"
	"sync"
	"time"

	"medics-health-check/backend/internal/monitoring"
)

type memoryMySQLRepository struct {
	mu      sync.RWMutex
	samples []monitoring.MySQLSample
	deltas  []monitoring.MySQLDelta
}

func newMemoryMySQLRepository(_ string) (*memoryMySQLRepository, error) {
	return &memoryMySQLRepository{}, nil
}

func (r *memoryMySQLRepository) AppendSample(sample monitoring.MySQLSample) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if sample.SampleID == "" {
		sample.SampleID = fmt.Sprintf("%s-%d", sample.CheckID, time.Now().UnixNano())
	}
	r.samples = append(r.samples, sample)
	return sample.SampleID, nil
}

func (r *memoryMySQLRepository) ComputeAndAppendDelta(sampleID string) (monitoring.MySQLDelta, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var current monitoring.MySQLSample
	found := false
	for _, sample := range r.samples {
		if sample.SampleID == sampleID {
			current = sample
			found = true
			break
		}
	}
	if !found {
		return monitoring.MySQLDelta{}, fmt.Errorf("sample not found: %s", sampleID)
	}

	var previous *monitoring.MySQLSample
	for i := len(r.samples) - 1; i >= 0; i-- {
		sample := r.samples[i]
		if sample.CheckID == current.CheckID && sample.SampleID != sampleID {
			previous = &sample
			break
		}
	}
	if previous == nil {
		return monitoring.MySQLDelta{}, fmt.Errorf("no previous sample for check %s", current.CheckID)
	}

	delta := monitoring.ComputeDelta(current, *previous)
	r.deltas = append(r.deltas, delta)
	return delta, nil
}

func (r *memoryMySQLRepository) LatestSample(checkID string) (monitoring.MySQLSample, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i := len(r.samples) - 1; i >= 0; i-- {
		if r.samples[i].CheckID == checkID {
			return r.samples[i], nil
		}
	}
	return monitoring.MySQLSample{}, fmt.Errorf("no samples found for check %s", checkID)
}

func (r *memoryMySQLRepository) RecentSamples(checkID string, limit int) ([]monitoring.MySQLSample, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}
	var out []monitoring.MySQLSample
	for i := len(r.samples) - 1; i >= 0 && len(out) < limit; i-- {
		if r.samples[i].CheckID == checkID {
			out = append(out, r.samples[i])
		}
	}
	return out, nil
}

func (r *memoryMySQLRepository) RecentDeltas(checkID string, limit int) ([]monitoring.MySQLDelta, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}
	var out []monitoring.MySQLDelta
	for i := len(r.deltas) - 1; i >= 0 && len(out) < limit; i-- {
		if r.deltas[i].CheckID == checkID {
			out = append(out, r.deltas[i])
		}
	}
	return out, nil
}

func (r *memoryMySQLRepository) PruneBefore(cutoff time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	samples := r.samples[:0]
	for _, sample := range r.samples {
		if !sample.Timestamp.Before(cutoff) {
			samples = append(samples, sample)
		}
	}
	r.samples = samples

	deltas := r.deltas[:0]
	for _, delta := range r.deltas {
		if !delta.Timestamp.Before(cutoff) {
			deltas = append(deltas, delta)
		}
	}
	r.deltas = deltas
	return nil
}

type memorySnapshotRepository struct {
	mu   sync.RWMutex
	data []monitoring.IncidentSnapshot
}

func newMemorySnapshotRepository(_ string) (*memorySnapshotRepository, error) {
	return &memorySnapshotRepository{}, nil
}

func (r *memorySnapshotRepository) SaveSnapshots(incidentID string, snaps []monitoring.IncidentSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, snap := range snaps {
		snap.IncidentID = incidentID
		r.data = append(r.data, snap)
	}
	return nil
}

func (r *memorySnapshotRepository) GetSnapshots(incidentID string) ([]monitoring.IncidentSnapshot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var out []monitoring.IncidentSnapshot
	for _, snap := range r.data {
		if snap.IncidentID == incidentID {
			out = append(out, snap)
		}
	}
	return out, nil
}

func (r *memorySnapshotRepository) PruneBefore(cutoff time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items := r.data[:0]
	for _, snap := range r.data {
		if !snap.Timestamp.Before(cutoff) {
			items = append(items, snap)
		}
	}
	r.data = items
	return nil
}

type memoryNotificationOutbox struct {
	mu   sync.RWMutex
	data []monitoring.NotificationEvent
}

func newMemoryNotificationOutbox(_ string) (*memoryNotificationOutbox, error) {
	return &memoryNotificationOutbox{}, nil
}

func (o *memoryNotificationOutbox) Enqueue(evt monitoring.NotificationEvent) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if evt.NotificationID == "" {
		evt.NotificationID = fmt.Sprintf("notif-%s-%d", evt.IncidentID, time.Now().UnixNano())
	}
	if evt.Status == "" {
		evt.Status = "pending"
	}
	if evt.CreatedAt.IsZero() {
		evt.CreatedAt = time.Now().UTC()
	}
	o.data = append(o.data, evt)
	return nil
}

func (o *memoryNotificationOutbox) ListPending(limit int) ([]monitoring.NotificationEvent, error) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}
	var out []monitoring.NotificationEvent
	for _, evt := range o.data {
		if evt.Status == "pending" {
			out = append(out, evt)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (o *memoryNotificationOutbox) MarkSent(id string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	for i := range o.data {
		if o.data[i].NotificationID == id {
			if o.data[i].Status != "pending" {
				return fmt.Errorf("notification %s is not pending (status: %s)", id, o.data[i].Status)
			}
			now := time.Now().UTC()
			o.data[i].Status = "sent"
			o.data[i].SentAt = &now
			return nil
		}
	}
	return fmt.Errorf("notification not found: %s", id)
}

func (o *memoryNotificationOutbox) MarkFailed(id string, reason string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	for i := range o.data {
		if o.data[i].NotificationID == id {
			o.data[i].Status = "failed"
			o.data[i].LastError = reason
			o.data[i].RetryCount++
			return nil
		}
	}
	return fmt.Errorf("notification not found: %s", id)
}

func (o *memoryNotificationOutbox) PruneBefore(cutoff time.Time) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	items := o.data[:0]
	for _, evt := range o.data {
		if !evt.CreatedAt.Before(cutoff) {
			items = append(items, evt)
		}
	}
	o.data = items
	return nil
}

func (o *memoryNotificationOutbox) AllNotifications() []monitoring.NotificationEvent {
	o.mu.RLock()
	defer o.mu.RUnlock()

	out := make([]monitoring.NotificationEvent, len(o.data))
	copy(out, o.data)
	return out
}

type memoryAIQueue struct {
	mu    sync.RWMutex
	items []monitoring.AIQueueItem
}

func newMemoryAIQueue(_ string) (*memoryAIQueue, error) {
	return &memoryAIQueue{}, nil
}

func (q *memoryAIQueue) Enqueue(incidentID string, promptVersion string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, item := range q.items {
		if item.IncidentID == incidentID && (item.Status == "pending" || item.Status == "processing") {
			return nil
		}
	}
	q.items = append(q.items, monitoring.AIQueueItem{
		IncidentID:    incidentID,
		PromptVersion: promptVersion,
		Status:        "pending",
		CreatedAt:     time.Now().UTC(),
	})
	return nil
}

func (q *memoryAIQueue) ClaimPending(limit int) ([]monitoring.AIQueueItem, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if limit <= 0 {
		limit = 10
	}
	var out []monitoring.AIQueueItem
	now := time.Now().UTC()
	for i := range q.items {
		if q.items[i].Status != "pending" {
			continue
		}
		q.items[i].Status = "processing"
		q.items[i].ClaimedAt = &now
		out = append(out, q.items[i])
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (q *memoryAIQueue) Complete(incidentID string, result monitoring.AIAnalysisResult) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i := range q.items {
		if q.items[i].IncidentID == incidentID && (q.items[i].Status == "pending" || q.items[i].Status == "processing") {
			now := time.Now().UTC()
			q.items[i].Status = "completed"
			q.items[i].CompletedAt = &now
			return nil
		}
	}
	return fmt.Errorf("no pending/processing AI queue item for incident %s", incidentID)
}

func (q *memoryAIQueue) Fail(incidentID string, reason string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i := range q.items {
		if q.items[i].IncidentID == incidentID && (q.items[i].Status == "pending" || q.items[i].Status == "processing") {
			q.items[i].Status = "failed"
			q.items[i].LastError = reason
			return nil
		}
	}
	return fmt.Errorf("no pending/processing AI queue item for incident %s", incidentID)
}

func (q *memoryAIQueue) PruneBefore(cutoff time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	items := q.items[:0]
	for _, item := range q.items {
		if !item.CreatedAt.Before(cutoff) {
			items = append(items, item)
		}
	}
	q.items = items
	return nil
}

func (q *memoryAIQueue) ListPendingItems(limit int) ([]monitoring.AIQueueItem, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}
	var out []monitoring.AIQueueItem
	for _, item := range q.items {
		if item.Status == "pending" {
			out = append(out, item)
			if len(out) >= limit {
				break
			}
		}
	}
	return out, nil
}

func (q *memoryAIQueue) AllItems() []monitoring.AIQueueItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	out := make([]monitoring.AIQueueItem, len(q.items))
	copy(out, q.items)
	return out
}
