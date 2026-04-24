package monitoring

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type FileStore struct {
	mu    sync.RWMutex
	path  string
	state State
}

var _ Store = (*FileStore)(nil)

func NewFileStore(path string, checks []CheckConfig) (*FileStore, error) {
	if path == "" {
		return nil, fmt.Errorf("state path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create state dir: %w", err)
	}

	store := &FileStore{path: path}
	stateExists := false
	if raw, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(raw, &store.state); err != nil {
			return nil, fmt.Errorf("parse state: %w", err)
		}
		stateExists = true
	}

	// Config checks are SEED ONLY: they populate state on first run when no
	// state file exists yet. After that, the persisted state is the single
	// source of truth — checks are managed exclusively via the API/UI and
	// survive restarts. This prevents config edits from silently deleting or
	// overwriting user-managed checks (including any added at runtime).
	if !stateExists && len(checks) > 0 {
		store.state.Checks = cloneChecks(checks)
	}
	store.state.UpdatedAt = time.Now().UTC()

	if err := store.flushLocked(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *FileStore) Snapshot() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneState(s.state)
}

func (s *FileStore) DashboardSnapshot() DashboardSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return buildDashboardSnapshot(s.state)
}

func (s *FileStore) Update(mutator func(*State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := cloneState(s.state)
	if err := mutator(&next); err != nil {
		return err
	}
	next.UpdatedAt = time.Now().UTC()
	if err := s.writeLocked(next); err != nil {
		return err
	}
	s.state = next
	return nil
}

func (s *FileStore) ReplaceChecks(checks []CheckConfig) error {
	return s.Update(func(state *State) error {
		state.Checks = cloneChecks(checks)
		return nil
	})
}

func (s *FileStore) UpsertCheck(check CheckConfig) error {
	return s.Update(func(state *State) error {
		for i := range state.Checks {
			if state.Checks[i].ID == check.ID {
				state.Checks[i] = check
				return nil
			}
		}
		state.Checks = append(state.Checks, check)
		return nil
	})
}

func (s *FileStore) DeleteCheck(id string) error {
	return s.Update(func(state *State) error {
		out := state.Checks[:0]
		for _, check := range state.Checks {
			if check.ID != id {
				out = append(out, check)
			}
		}
		state.Checks = out
		return nil
	})
}

func (s *FileStore) AppendResults(results []CheckResult, retentionDays int) error {
	return s.Update(func(state *State) error {
		state.Results = append(state.Results, results...)
		pruneResults(&state.Results, retentionDays)
		return nil
	})
}

func (s *FileStore) SetLastRun(at time.Time) error {
	return s.Update(func(state *State) error {
		state.LastRunAt = at.UTC()
		return nil
	})
}

func (s *FileStore) writeLocked(state State) error {
	encoded, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, encoded, 0o600); err != nil {
		return fmt.Errorf("write temp state: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("replace state: %w", err)
	}
	return nil
}

func (s *FileStore) flushLocked() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeLocked(s.state)
}

func cloneState(state State) State {
	return State{
		Checks:    cloneChecks(state.Checks),
		Results:   cloneResults(state.Results),
		LastRunAt: state.LastRunAt,
		UpdatedAt: state.UpdatedAt,
	}
}

func cloneChecks(checks []CheckConfig) []CheckConfig {
	if len(checks) == 0 {
		return nil
	}
	out := make([]CheckConfig, len(checks))
	copy(out, checks)
	return out
}

func cloneResults(results []CheckResult) []CheckResult {
	if len(results) == 0 {
		return nil
	}
	out := make([]CheckResult, len(results))
	for i := range results {
		out[i] = results[i]
		if results[i].Metrics != nil {
			metrics := make(map[string]float64, len(results[i].Metrics))
			for k, v := range results[i].Metrics {
				metrics[k] = v
			}
			out[i].Metrics = metrics
		}
		if results[i].Tags != nil {
			tags := make([]string, len(results[i].Tags))
			copy(tags, results[i].Tags)
			out[i].Tags = tags
		}
	}
	return out
}

func pruneResults(results *[]CheckResult, retentionDays int) {
	if retentionDays <= 0 {
		return
	}
	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	items := (*results)[:0]
	for _, result := range *results {
		finishedAt := result.FinishedAt
		if finishedAt.IsZero() {
			finishedAt = result.StartedAt
		}
		if finishedAt.IsZero() || finishedAt.After(cutoff) {
			items = append(items, result)
		}
	}
	*results = items
	sort.SliceStable(*results, func(i, j int) bool {
		return (*results)[i].FinishedAt.Before((*results)[j].FinishedAt)
	})
}
