package mysql

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"medics-health-check/backend/internal/monitoring"
	"medics-health-check/backend/internal/util/jsonl"
)

// FileMySQLRepository implements monitoring.MySQLMetricsRepository with JSONL file backing.
type FileMySQLRepository struct {
	mu         sync.RWMutex
	samplesDir string
	deltasDir  string
	samples    []monitoring.MySQLSample
	deltas     []monitoring.MySQLDelta
}

// NewFileMySQLRepository creates a new file-backed MySQL metrics repository.
func NewFileMySQLRepository(dataDir string) (*FileMySQLRepository, error) {
	samplesPath := filepath.Join(dataDir, "mysql_samples.jsonl")
	deltasPath := filepath.Join(dataDir, "mysql_deltas.jsonl")

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create mysql data dir: %w", err)
	}

	repo := &FileMySQLRepository{
		samplesDir: samplesPath,
		deltasDir:  deltasPath,
	}

	var err error
	repo.samples, err = jsonl.Load[monitoring.MySQLSample](samplesPath)
	if err != nil {
		return nil, fmt.Errorf("load samples: %w", err)
	}

	repo.deltas, err = jsonl.Load[monitoring.MySQLDelta](deltasPath)
	if err != nil {
		return nil, fmt.Errorf("load deltas: %w", err)
	}

	return repo, nil
}

func (r *FileMySQLRepository) AppendSample(sample monitoring.MySQLSample) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if sample.SampleID == "" {
		sample.SampleID = fmt.Sprintf("%s-%d", sample.CheckID, time.Now().UnixNano())
	}

	r.samples = append(r.samples, sample)
	if err := jsonl.Append(r.samplesDir, sample); err != nil {
		return "", fmt.Errorf("append sample: %w", err)
	}
	return sample.SampleID, nil
}

func (r *FileMySQLRepository) ComputeAndAppendDelta(sampleID string) (monitoring.MySQLDelta, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var current monitoring.MySQLSample
	found := false
	for _, s := range r.samples {
		if s.SampleID == sampleID {
			current = s
			found = true
			break
		}
	}
	if !found {
		return monitoring.MySQLDelta{}, fmt.Errorf("sample not found: %s", sampleID)
	}

	// Find previous sample for same check
	var previous *monitoring.MySQLSample
	for i := len(r.samples) - 1; i >= 0; i-- {
		s := r.samples[i]
		if s.CheckID == current.CheckID && s.SampleID != sampleID {
			previous = &s
			break
		}
	}
	if previous == nil {
		return monitoring.MySQLDelta{}, fmt.Errorf("no previous sample for check %s", current.CheckID)
	}

	delta := monitoring.ComputeDelta(current, *previous)

	r.deltas = append(r.deltas, delta)
	if err := jsonl.Append(r.deltasDir, delta); err != nil {
		return monitoring.MySQLDelta{}, fmt.Errorf("append delta: %w", err)
	}
	return delta, nil
}

func (r *FileMySQLRepository) LatestSample(checkID string) (monitoring.MySQLSample, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i := len(r.samples) - 1; i >= 0; i-- {
		if r.samples[i].CheckID == checkID {
			return r.samples[i], nil
		}
	}
	return monitoring.MySQLSample{}, fmt.Errorf("no samples found for check %s", checkID)
}

func (r *FileMySQLRepository) RecentSamples(checkID string, limit int) ([]monitoring.MySQLSample, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	var result []monitoring.MySQLSample
	for i := len(r.samples) - 1; i >= 0 && len(result) < limit; i-- {
		if r.samples[i].CheckID == checkID {
			result = append(result, r.samples[i])
		}
	}
	return result, nil
}

func (r *FileMySQLRepository) RecentDeltas(checkID string, limit int) ([]monitoring.MySQLDelta, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	var result []monitoring.MySQLDelta
	for i := len(r.deltas) - 1; i >= 0 && len(result) < limit; i-- {
		if r.deltas[i].CheckID == checkID {
			result = append(result, r.deltas[i])
		}
	}
	return result, nil
}

// PruneBefore removes samples and deltas older than the given cutoff.
func (r *FileMySQLRepository) PruneBefore(cutoff time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	pruned := r.samples[:0]
	for _, s := range r.samples {
		if !s.Timestamp.Before(cutoff) {
			pruned = append(pruned, s)
		}
	}
	r.samples = pruned

	prunedDeltas := r.deltas[:0]
	for _, d := range r.deltas {
		if !d.Timestamp.Before(cutoff) {
			prunedDeltas = append(prunedDeltas, d)
		}
	}
	r.deltas = prunedDeltas

	if err := jsonl.Rewrite(r.samplesDir, r.samples); err != nil {
		return fmt.Errorf("rewrite samples: %w", err)
	}
	if err := jsonl.Rewrite(r.deltasDir, r.deltas); err != nil {
		return fmt.Errorf("rewrite deltas: %w", err)
	}
	return nil
}
