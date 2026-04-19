package monitoring

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AIQueueRepository defines generic AI analysis queue operations.
// Reusable across all check types, not MySQL-specific.
type AIQueueRepository interface {
	Enqueue(incidentID string, promptVersion string) error
	ClaimPending(limit int) ([]AIQueueItem, error)
	Complete(incidentID string, result AIAnalysisResult) error
	Fail(incidentID string, reason string) error
	PruneBefore(cutoff time.Time) error
}

// FileAIQueue implements AIQueueRepository with JSONL file backing.
type FileAIQueue struct {
	mu          sync.RWMutex
	queuePath   string
	resultsPath string
	queue       []AIQueueItem
	results     []AIAnalysisResult
}

// NewFileAIQueue creates a file-backed AI queue.
func NewFileAIQueue(dataDir string) (*FileAIQueue, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create ai queue dir: %w", err)
	}

	queuePath := filepath.Join(dataDir, "ai_queue.jsonl")
	resultsPath := filepath.Join(dataDir, "ai_results.jsonl")

	q := &FileAIQueue{
		queuePath:   queuePath,
		resultsPath: resultsPath,
	}

	var err error
	q.queue, err = loadJSONLFile[AIQueueItem](queuePath)
	if err != nil {
		return nil, fmt.Errorf("load ai queue: %w", err)
	}

	q.results, err = loadJSONLFile[AIAnalysisResult](resultsPath)
	if err != nil {
		return nil, fmt.Errorf("load ai results: %w", err)
	}

	return q, nil
}

func (q *FileAIQueue) Enqueue(incidentID string, promptVersion string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Dedup: don't enqueue if there's already a pending/processing item for this incident
	for _, item := range q.queue {
		if item.IncidentID == incidentID && (item.Status == "pending" || item.Status == "processing") {
			return nil // idempotent
		}
	}

	item := AIQueueItem{
		IncidentID:    incidentID,
		PromptVersion: promptVersion,
		Status:        "pending",
		CreatedAt:     time.Now().UTC(),
	}

	q.queue = append(q.queue, item)
	return appendJSONLFile(q.queuePath, item)
}

func (q *FileAIQueue) ClaimPending(limit int) ([]AIQueueItem, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if limit <= 0 {
		limit = 10
	}

	var claimed []AIQueueItem
	for i := range q.queue {
		if q.queue[i].Status == "pending" {
			now := time.Now().UTC()
			q.queue[i].Status = "processing"
			q.queue[i].ClaimedAt = &now
			claimed = append(claimed, q.queue[i])
			if len(claimed) >= limit {
				break
			}
		}
	}

	if len(claimed) > 0 {
		if err := rewriteJSONLFile(q.queuePath, q.queue); err != nil {
			return nil, fmt.Errorf("persist claimed items: %w", err)
		}
	}

	return claimed, nil
}

func (q *FileAIQueue) Complete(incidentID string, result AIAnalysisResult) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	found := false
	for i := range q.queue {
		if q.queue[i].IncidentID == incidentID && (q.queue[i].Status == "processing" || q.queue[i].Status == "pending") {
			now := time.Now().UTC()
			q.queue[i].Status = "completed"
			q.queue[i].CompletedAt = &now
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("no pending/processing AI queue item for incident %s", incidentID)
	}

	result.IncidentID = incidentID
	if result.CreatedAt.IsZero() {
		result.CreatedAt = time.Now().UTC()
	}

	q.results = append(q.results, result)

	if err := rewriteJSONLFile(q.queuePath, q.queue); err != nil {
		return fmt.Errorf("persist queue: %w", err)
	}
	if err := appendJSONLFile(q.resultsPath, result); err != nil {
		return fmt.Errorf("persist result: %w", err)
	}
	return nil
}

func (q *FileAIQueue) Fail(incidentID string, reason string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	found := false
	for i := range q.queue {
		if q.queue[i].IncidentID == incidentID && (q.queue[i].Status == "processing" || q.queue[i].Status == "pending") {
			q.queue[i].Status = "failed"
			q.queue[i].LastError = reason
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("no pending/processing AI queue item for incident %s", incidentID)
	}

	return rewriteJSONLFile(q.queuePath, q.queue)
}

func (q *FileAIQueue) PruneBefore(cutoff time.Time) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	pruned := q.queue[:0]
	for _, item := range q.queue {
		if !item.CreatedAt.Before(cutoff) {
			pruned = append(pruned, item)
		}
	}
	q.queue = pruned

	prunedResults := q.results[:0]
	for _, r := range q.results {
		if !r.CreatedAt.Before(cutoff) {
			prunedResults = append(prunedResults, r)
		}
	}
	q.results = prunedResults

	if err := rewriteJSONLFile(q.queuePath, q.queue); err != nil {
		return err
	}
	return rewriteJSONLFile(q.resultsPath, q.results)
}

// GetResults returns AI analysis results for a specific incident.
func (q *FileAIQueue) GetResults(incidentID string) []AIAnalysisResult {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var out []AIAnalysisResult
	for _, r := range q.results {
		if r.IncidentID == incidentID {
			out = append(out, r)
		}
	}
	return out
}

// ListPendingItems returns pending AI queue items (for API read access).
func (q *FileAIQueue) ListPendingItems(limit int) ([]AIQueueItem, error) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	var result []AIQueueItem
	for _, item := range q.queue {
		if item.Status == "pending" {
			result = append(result, item)
			if len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}
