package monitoring

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileAIQueue_EnqueueAndListPending(t *testing.T) {
	dir := t.TempDir()
	q, err := NewFileAIQueue(dir)
	if err != nil {
		t.Fatalf("create queue: %v", err)
	}

	if err := q.Enqueue("inc-1", "v1"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	items, err := q.ListPendingItems(10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 pending, got %d", len(items))
	}
	if items[0].IncidentID != "inc-1" {
		t.Errorf("expected inc-1, got %s", items[0].IncidentID)
	}
	if items[0].Status != "pending" {
		t.Errorf("expected pending, got %s", items[0].Status)
	}
}

func TestFileAIQueue_Dedup(t *testing.T) {
	dir := t.TempDir()
	q, _ := NewFileAIQueue(dir)

	_ = q.Enqueue("inc-1", "v1")
	_ = q.Enqueue("inc-1", "v1") // duplicate

	items, _ := q.ListPendingItems(10)
	if len(items) != 1 {
		t.Errorf("expected dedup to 1, got %d", len(items))
	}
}

func TestFileAIQueue_ClaimPending(t *testing.T) {
	dir := t.TempDir()
	q, _ := NewFileAIQueue(dir)

	_ = q.Enqueue("inc-1", "v1")
	_ = q.Enqueue("inc-2", "v1")

	claimed, err := q.ClaimPending(1)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("expected 1 claimed, got %d", len(claimed))
	}
	if claimed[0].Status != "processing" {
		t.Errorf("expected processing, got %s", claimed[0].Status)
	}
	if claimed[0].ClaimedAt == nil {
		t.Error("expected ClaimedAt to be set")
	}

	// Only 1 should remain pending
	pending, _ := q.ListPendingItems(10)
	if len(pending) != 1 {
		t.Errorf("expected 1 pending after claim, got %d", len(pending))
	}
}

func TestFileAIQueue_Complete(t *testing.T) {
	dir := t.TempDir()
	q, _ := NewFileAIQueue(dir)

	_ = q.Enqueue("inc-1", "v1")
	_, _ = q.ClaimPending(1)

	result := AIAnalysisResult{
		Analysis:    "Root cause: connection leak",
		Suggestions: []string{"Increase pool size", "Fix connection leak"},
		Severity:    "critical",
	}

	if err := q.Complete("inc-1", result); err != nil {
		t.Fatalf("complete: %v", err)
	}

	// Should no longer be pending
	pending, _ := q.ListPendingItems(10)
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after complete, got %d", len(pending))
	}
}

func TestFileAIQueue_CompleteNotFound(t *testing.T) {
	dir := t.TempDir()
	q, _ := NewFileAIQueue(dir)

	err := q.Complete("nonexistent", AIAnalysisResult{})
	if err == nil {
		t.Fatal("expected error for nonexistent incident")
	}
}

func TestFileAIQueue_Fail(t *testing.T) {
	dir := t.TempDir()
	q, _ := NewFileAIQueue(dir)

	_ = q.Enqueue("inc-1", "v1")
	_, _ = q.ClaimPending(1)

	if err := q.Fail("inc-1", "API timeout"); err != nil {
		t.Fatalf("fail: %v", err)
	}

	pending, _ := q.ListPendingItems(10)
	if len(pending) != 0 {
		t.Errorf("expected 0 pending after fail, got %d", len(pending))
	}
}

func TestFileAIQueue_FailNotFound(t *testing.T) {
	dir := t.TempDir()
	q, _ := NewFileAIQueue(dir)

	err := q.Fail("nonexistent", "reason")
	if err == nil {
		t.Fatal("expected error for nonexistent incident")
	}
}

func TestFileAIQueue_PruneBefore(t *testing.T) {
	dir := t.TempDir()
	q, _ := NewFileAIQueue(dir)

	now := time.Now().UTC()

	// Enqueue with old timestamp
	q.mu.Lock()
	oldItem := AIQueueItem{
		IncidentID: "old", PromptVersion: "v1",
		Status: "completed", CreatedAt: now.Add(-48 * time.Hour),
	}
	q.queue = append(q.queue, oldItem)
	_ = appendJSONLFile(q.queuePath, oldItem)
	q.mu.Unlock()

	_ = q.Enqueue("new", "v1") // recent

	cutoff := now.Add(-24 * time.Hour)
	if err := q.PruneBefore(cutoff); err != nil {
		t.Fatalf("prune: %v", err)
	}

	items, _ := q.ListPendingItems(10)
	if len(items) != 1 {
		t.Errorf("expected 1 item after prune, got %d", len(items))
	}
}

func TestFileAIQueue_Persistence(t *testing.T) {
	dir := t.TempDir()

	q1, _ := NewFileAIQueue(dir)
	_ = q1.Enqueue("inc-persist", "v1")

	// Verify file exists
	queuePath := filepath.Join(dir, "ai_queue.jsonl")
	if _, err := os.Stat(queuePath); os.IsNotExist(err) {
		t.Fatal("queue JSONL file not created")
	}

	// Reload
	q2, err := NewFileAIQueue(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	items, _ := q2.ListPendingItems(10)
	if len(items) != 1 {
		t.Fatalf("expected 1 pending after reload, got %d", len(items))
	}
	if items[0].IncidentID != "inc-persist" {
		t.Errorf("expected inc-persist, got %s", items[0].IncidentID)
	}
}

func TestFileAIQueue_ListPendingDefaultLimit(t *testing.T) {
	dir := t.TempDir()
	q, _ := NewFileAIQueue(dir)

	// limit=0 should default to 100
	items, err := q.ListPendingItems(0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if items != nil && len(items) != 0 {
		t.Errorf("expected empty list, got %d items", len(items))
	}
}

func TestFileAIQueue_DedupAllowsAfterCompletion(t *testing.T) {
	dir := t.TempDir()
	q, _ := NewFileAIQueue(dir)

	_ = q.Enqueue("inc-1", "v1")
	_, _ = q.ClaimPending(1)
	_ = q.Complete("inc-1", AIAnalysisResult{Analysis: "done"})

	// Should be able to enqueue again after completion
	_ = q.Enqueue("inc-1", "v2")

	items, _ := q.ListPendingItems(10)
	if len(items) != 1 {
		t.Errorf("expected re-enqueue after completion, got %d", len(items))
	}
}
