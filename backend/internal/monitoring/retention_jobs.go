package monitoring

import (
	"log"
	"time"
)

// RetentionConfig holds configurable retention windows.
type RetentionConfig struct {
	SampleRetentionDays       int `json:"sampleRetentionDays"`
	DeltaRetentionDays        int `json:"deltaRetentionDays"`
	SnapshotRetentionDays     int `json:"snapshotRetentionDays"`
	NotificationRetentionDays int `json:"notificationRetentionDays"`
	AIQueueRetentionDays      int `json:"aiQueueRetentionDays"`
	IncidentRetentionDays     int `json:"incidentRetentionDays"`
}

// DefaultRetentionConfig returns sensible default retention windows.
func DefaultRetentionConfig() RetentionConfig {
	return RetentionConfig{
		SampleRetentionDays:       7,
		DeltaRetentionDays:        7,
		SnapshotRetentionDays:     30,
		NotificationRetentionDays: 14,
		AIQueueRetentionDays:      30,
		IncidentRetentionDays:     90,
	}
}

// Prunable is any repository that supports time-based pruning. Generic interface.
type Prunable interface {
	PruneBefore(cutoff time.Time) error
}

// RetentionJob runs daily cleanup across all prunable repositories.
// Generic — works with any combination of repositories, not tied to MySQL.
type RetentionJob struct {
	config RetentionConfig
	logger *log.Logger
	repos  map[string]prunableEntry
}

type prunableEntry struct {
	repo          Prunable
	retentionDays int
}

// NewRetentionJob creates a retention job with the given config.
func NewRetentionJob(config RetentionConfig, logger *log.Logger) *RetentionJob {
	if logger == nil {
		logger = log.Default()
	}
	return &RetentionJob{
		config: config,
		logger: logger,
		repos:  make(map[string]prunableEntry),
	}
}

// Register adds a prunable repository to the retention job.
func (j *RetentionJob) Register(name string, repo Prunable, retentionDays int) {
	if retentionDays <= 0 {
		retentionDays = 7
	}
	j.repos[name] = prunableEntry{repo: repo, retentionDays: retentionDays}
}

// RunOnce executes a single retention pass across all registered repositories.
func (j *RetentionJob) RunOnce() {
	now := time.Now().UTC()

	for name, entry := range j.repos {
		cutoff := now.Add(-time.Duration(entry.retentionDays) * 24 * time.Hour)
		if err := entry.repo.PruneBefore(cutoff); err != nil {
			j.logger.Printf("retention cleanup failed for %s: %v", name, err)
		} else {
			j.logger.Printf("retention cleanup completed for %s (cutoff: %v)", name, cutoff)
		}
	}
}

// RunDaily starts a background goroutine that runs cleanup once per 24 hours.
// Returns a stop function to cancel the background job.
func (j *RetentionJob) RunDaily(stop <-chan struct{}) {
	go func() {
		// Run immediately on start
		j.RunOnce()

		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				j.RunOnce()
			}
		}
	}()
}
