package monitoring

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// MySQLMetricsRepository defines persistence for MySQL samples and deltas.
type MySQLMetricsRepository interface {
	AppendSample(sample MySQLSample) (string, error)
	ComputeAndAppendDelta(sampleID string) (MySQLDelta, error)
	LatestSample(checkID string) (MySQLSample, error)
	RecentSamples(checkID string, limit int) ([]MySQLSample, error)
	RecentDeltas(checkID string, limit int) ([]MySQLDelta, error)
}

// FileMySQLRepository implements MySQLMetricsRepository with JSONL file backing.
type FileMySQLRepository struct {
	mu         sync.RWMutex
	samplesDir string
	deltasDir  string
	samples    []MySQLSample
	deltas     []MySQLDelta
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
	repo.samples, err = loadJSONLFile[MySQLSample](samplesPath)
	if err != nil {
		return nil, fmt.Errorf("load samples: %w", err)
	}

	repo.deltas, err = loadJSONLFile[MySQLDelta](deltasPath)
	if err != nil {
		return nil, fmt.Errorf("load deltas: %w", err)
	}

	return repo, nil
}

func (r *FileMySQLRepository) AppendSample(sample MySQLSample) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if sample.SampleID == "" {
		sample.SampleID = fmt.Sprintf("%s-%d", sample.CheckID, time.Now().UnixNano())
	}

	r.samples = append(r.samples, sample)
	if err := appendJSONLFile(r.samplesDir, sample); err != nil {
		return "", fmt.Errorf("append sample: %w", err)
	}
	return sample.SampleID, nil
}

func (r *FileMySQLRepository) ComputeAndAppendDelta(sampleID string) (MySQLDelta, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var current MySQLSample
	found := false
	for _, s := range r.samples {
		if s.SampleID == sampleID {
			current = s
			found = true
			break
		}
	}
	if !found {
		return MySQLDelta{}, fmt.Errorf("sample not found: %s", sampleID)
	}

	// Find previous sample for same check
	var previous *MySQLSample
	for i := len(r.samples) - 1; i >= 0; i-- {
		s := r.samples[i]
		if s.CheckID == current.CheckID && s.SampleID != sampleID {
			previous = &s
			break
		}
	}
	if previous == nil {
		return MySQLDelta{}, fmt.Errorf("no previous sample for check %s", current.CheckID)
	}

	delta := ComputeDelta(current, *previous)

	r.deltas = append(r.deltas, delta)
	if err := appendJSONLFile(r.deltasDir, delta); err != nil {
		return MySQLDelta{}, fmt.Errorf("append delta: %w", err)
	}
	return delta, nil
}

func (r *FileMySQLRepository) LatestSample(checkID string) (MySQLSample, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for i := len(r.samples) - 1; i >= 0; i-- {
		if r.samples[i].CheckID == checkID {
			return r.samples[i], nil
		}
	}
	return MySQLSample{}, fmt.Errorf("no samples found for check %s", checkID)
}

func (r *FileMySQLRepository) RecentSamples(checkID string, limit int) ([]MySQLSample, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	var result []MySQLSample
	for i := len(r.samples) - 1; i >= 0 && len(result) < limit; i-- {
		if r.samples[i].CheckID == checkID {
			result = append(result, r.samples[i])
		}
	}
	return result, nil
}

func (r *FileMySQLRepository) RecentDeltas(checkID string, limit int) ([]MySQLDelta, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}

	var result []MySQLDelta
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

	if err := rewriteJSONLFile(r.samplesDir, r.samples); err != nil {
		return fmt.Errorf("rewrite samples: %w", err)
	}
	if err := rewriteJSONLFile(r.deltasDir, r.deltas); err != nil {
		return fmt.Errorf("rewrite deltas: %w", err)
	}
	return nil
}

// --- JSONL helpers (generic, reusable) ---

func loadJSONLFile[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var items []T
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var item T
		if err := json.Unmarshal(line, &item); err != nil {
			continue // skip malformed lines
		}
		items = append(items, item)
	}
	return items, scanner.Err()
}

func appendJSONLFile[T any](path string, item T) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = f.Write(data)
	return err
}

func rewriteJSONLFile[T any](path string, items []T) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	writer := bufio.NewWriter(f)
	for _, item := range items {
		data, err := json.Marshal(item)
		if err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
		writer.Write(data)
		writer.WriteByte('\n')
	}

	if err := writer.Flush(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}
