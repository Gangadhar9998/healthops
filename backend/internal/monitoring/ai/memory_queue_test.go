package ai

import (
	"fmt"
	"sync"
	"time"

	"medics-health-check/backend/internal/monitoring"
)

type memoryAIQueue struct {
	mu      sync.RWMutex
	queue   []monitoring.AIQueueItem
	results []monitoring.AIAnalysisResult
}

func newMemoryAIQueue() *memoryAIQueue {
	return &memoryAIQueue{}
}

func (q *memoryAIQueue) Enqueue(incidentID string, promptVersion string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for _, item := range q.queue {
		if item.IncidentID == incidentID && (item.Status == "pending" || item.Status == "processing") {
			return nil
		}
	}
	q.queue = append(q.queue, monitoring.AIQueueItem{
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
	var claimed []monitoring.AIQueueItem
	now := time.Now().UTC()
	for i := range q.queue {
		if q.queue[i].Status != "pending" {
			continue
		}
		q.queue[i].Status = "processing"
		q.queue[i].ClaimedAt = &now
		claimed = append(claimed, q.queue[i])
		if len(claimed) >= limit {
			break
		}
	}
	return claimed, nil
}

func (q *memoryAIQueue) Complete(incidentID string, result monitoring.AIAnalysisResult) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i := range q.queue {
		if q.queue[i].IncidentID == incidentID && (q.queue[i].Status == "pending" || q.queue[i].Status == "processing") {
			now := time.Now().UTC()
			q.queue[i].Status = "completed"
			q.queue[i].CompletedAt = &now
			result.IncidentID = incidentID
			if result.CreatedAt.IsZero() {
				result.CreatedAt = now
			}
			q.results = append(q.results, result)
			return nil
		}
	}
	return fmt.Errorf("no pending/processing AI queue item for incident %s", incidentID)
}

func (q *memoryAIQueue) Fail(incidentID string, reason string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i := range q.queue {
		if q.queue[i].IncidentID == incidentID && (q.queue[i].Status == "pending" || q.queue[i].Status == "processing") {
			q.queue[i].Status = "failed"
			q.queue[i].LastError = reason
			return nil
		}
	}
	return fmt.Errorf("no pending/processing AI queue item for incident %s", incidentID)
}

func (q *memoryAIQueue) PruneBefore(cutoff time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	items := q.queue[:0]
	for _, item := range q.queue {
		if !item.CreatedAt.Before(cutoff) {
			items = append(items, item)
		}
	}
	q.queue = items

	results := q.results[:0]
	for _, result := range q.results {
		if !result.CreatedAt.Before(cutoff) {
			results = append(results, result)
		}
	}
	q.results = results
	return nil
}

func (q *memoryAIQueue) GetResults(incidentID string) []monitoring.AIAnalysisResult {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var out []monitoring.AIAnalysisResult
	for _, result := range q.results {
		if result.IncidentID == incidentID {
			out = append(out, result)
		}
	}
	return out
}

func (q *memoryAIQueue) AllResults(limit int) []monitoring.AIAnalysisResult {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}
	n := len(q.results)
	if n > limit {
		n = limit
	}
	out := make([]monitoring.AIAnalysisResult, n)
	for i := 0; i < n; i++ {
		out[i] = q.results[len(q.results)-1-i]
	}
	return out
}

func (q *memoryAIQueue) ListPendingItems(limit int) ([]monitoring.AIQueueItem, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}
	var out []monitoring.AIQueueItem
	for _, item := range q.queue {
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

	out := make([]monitoring.AIQueueItem, len(q.queue))
	copy(out, q.queue)
	return out
}
