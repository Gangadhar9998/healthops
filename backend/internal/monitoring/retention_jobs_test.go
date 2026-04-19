package monitoring

import (
	"bytes"
	"fmt"
	"log"
	"testing"
	"time"
)

func TestRetentionJob_RunOnce(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	job := NewRetentionJob(DefaultRetentionConfig(), logger)

	pruned := false
	mock := &mockPrunable{
		pruneFunc: func(cutoff time.Time) error {
			pruned = true
			return nil
		},
	}

	job.Register("test", mock, 7)
	job.RunOnce()

	if !pruned {
		t.Error("expected prune to be called")
	}

	if !bytes.Contains(buf.Bytes(), []byte("retention cleanup completed for test")) {
		t.Errorf("expected success log, got: %s", buf.String())
	}
}

func TestRetentionJob_RunOnceError(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	job := NewRetentionJob(DefaultRetentionConfig(), logger)

	mock := &mockPrunable{
		pruneFunc: func(cutoff time.Time) error {
			return errTest
		},
	}

	job.Register("failing", mock, 7)
	job.RunOnce()

	if !bytes.Contains(buf.Bytes(), []byte("retention cleanup failed for failing")) {
		t.Errorf("expected failure log, got: %s", buf.String())
	}
}

func TestRetentionJob_CutoffCalculation(t *testing.T) {
	logger := newTestLogger()
	job := NewRetentionJob(DefaultRetentionConfig(), logger)

	var capturedCutoff time.Time
	mock := &mockPrunable{
		pruneFunc: func(cutoff time.Time) error {
			capturedCutoff = cutoff
			return nil
		},
	}

	job.Register("test", mock, 30)
	before := time.Now().UTC()
	job.RunOnce()

	expectedCutoff := before.Add(-30 * 24 * time.Hour)
	diff := capturedCutoff.Sub(expectedCutoff)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("cutoff %v too far from expected %v", capturedCutoff, expectedCutoff)
	}
}

func TestRetentionJob_DefaultRetentionDays(t *testing.T) {
	logger := newTestLogger()
	job := NewRetentionJob(DefaultRetentionConfig(), logger)

	var capturedCutoff time.Time
	mock := &mockPrunable{
		pruneFunc: func(cutoff time.Time) error {
			capturedCutoff = cutoff
			return nil
		},
	}

	// Register with 0 retention days — should default to 7
	job.Register("default", mock, 0)
	before := time.Now().UTC()
	job.RunOnce()

	expectedCutoff := before.Add(-7 * 24 * time.Hour)
	diff := capturedCutoff.Sub(expectedCutoff)
	if diff < -time.Second || diff > time.Second {
		t.Errorf("expected 7-day default cutoff, got diff %v", diff)
	}
}

func TestRetentionJob_MultipleRepos(t *testing.T) {
	logger := newTestLogger()
	job := NewRetentionJob(DefaultRetentionConfig(), logger)

	prunedNames := make(map[string]bool)
	for _, name := range []string{"a", "b", "c"} {
		n := name
		job.Register(n, &mockPrunable{
			pruneFunc: func(cutoff time.Time) error {
				prunedNames[n] = true
				return nil
			},
		}, 7)
	}

	job.RunOnce()

	for _, name := range []string{"a", "b", "c"} {
		if !prunedNames[name] {
			t.Errorf("expected %s to be pruned", name)
		}
	}
}

func TestRetentionJob_RunDaily(t *testing.T) {
	logger := newTestLogger()
	job := NewRetentionJob(DefaultRetentionConfig(), logger)

	called := make(chan struct{}, 1)
	job.Register("test", &mockPrunable{
		pruneFunc: func(cutoff time.Time) error {
			select {
			case called <- struct{}{}:
			default:
			}
			return nil
		},
	}, 7)

	stop := make(chan struct{})
	job.RunDaily(stop)

	// Should run immediately
	select {
	case <-called:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("RunDaily did not execute initial run within timeout")
	}

	close(stop)
}

func TestDefaultRetentionConfig(t *testing.T) {
	cfg := DefaultRetentionConfig()

	if cfg.SampleRetentionDays != 7 {
		t.Errorf("expected sample retention 7, got %d", cfg.SampleRetentionDays)
	}
	if cfg.SnapshotRetentionDays != 30 {
		t.Errorf("expected snapshot retention 30, got %d", cfg.SnapshotRetentionDays)
	}
	if cfg.IncidentRetentionDays != 90 {
		t.Errorf("expected incident retention 90, got %d", cfg.IncidentRetentionDays)
	}
}

// Mock implementations

type mockPrunable struct {
	pruneFunc func(cutoff time.Time) error
}

func (m *mockPrunable) PruneBefore(cutoff time.Time) error {
	if m.pruneFunc != nil {
		return m.pruneFunc(cutoff)
	}
	return nil
}

var errTest = fmt.Errorf("test error")
