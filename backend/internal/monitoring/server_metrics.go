package monitoring

import (
	"medics-health-check/backend/internal/util/jsonl"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// ServerSnapshot captures a point-in-time snapshot of server metrics.
type ServerSnapshot struct {
	ServerID           string        `json:"serverId"`
	Timestamp          time.Time     `json:"timestamp"`
	CPUUsagePercent    float64       `json:"cpuPercent"`
	MemoryTotalMB      float64       `json:"memoryTotalMB"`
	MemoryUsedMB       float64       `json:"memoryUsedMB"`
	MemoryUsagePercent float64       `json:"memoryPercent"`
	DiskTotalGB        float64       `json:"diskTotalGB"`
	DiskUsedGB         float64       `json:"diskUsedGB"`
	DiskUsagePercent   float64       `json:"diskPercent"`
	LoadAvg1           float64       `json:"loadAvg1"`
	LoadAvg5           float64       `json:"loadAvg5"`
	LoadAvg15          float64       `json:"loadAvg15"`
	UptimeHours        float64       `json:"uptimeHours"`
	TopProcesses       []ProcessInfo `json:"topProcesses,omitempty"`
}

// ServerMetricsRepository stores server metric snapshots in JSONL files.
type ServerMetricsRepository struct {
	dir string
	mu  sync.RWMutex
}

// NewServerMetricsRepository creates a new repository for server metrics.
func NewServerMetricsRepository(dataDir string) *ServerMetricsRepository {
	dir := filepath.Join(dataDir, "server_metrics")
	_ = os.MkdirAll(dir, 0700)
	return &ServerMetricsRepository{dir: dir}
}

func (r *ServerMetricsRepository) filePath(serverID string) string {
	return filepath.Join(r.dir, serverID+".jsonl")
}

// Save stores a server snapshot.
func (r *ServerMetricsRepository) Save(snap ServerSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return jsonl.Append(r.filePath(snap.ServerID), snap)
}

// GetSnapshots returns snapshots for a server within a time range.
func (r *ServerMetricsRepository) GetSnapshots(serverID string, since, until time.Time) ([]ServerSnapshot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all, err := jsonl.Load[ServerSnapshot](r.filePath(serverID))
	if err != nil {
		return nil, err
	}

	var filtered []ServerSnapshot
	for _, s := range all {
		if (since.IsZero() || !s.Timestamp.Before(since)) &&
			(until.IsZero() || !s.Timestamp.After(until)) {
			filtered = append(filtered, s)
		}
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.Before(filtered[j].Timestamp)
	})

	return filtered, nil
}

// GetLatest returns the most recent snapshot for a server.
func (r *ServerMetricsRepository) GetLatest(serverID string) (*ServerSnapshot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all, err := jsonl.Load[ServerSnapshot](r.filePath(serverID))
	if err != nil {
		return nil, err
	}
	if len(all) == 0 {
		return nil, nil
	}

	latest := all[0]
	for _, s := range all[1:] {
		if s.Timestamp.After(latest.Timestamp) {
			latest = s
		}
	}
	return &latest, nil
}

// Prune removes snapshots older than the cutoff.
func (r *ServerMetricsRepository) Prune(cutoff time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entries, err := os.ReadDir(r.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	totalPruned := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(r.dir, e.Name())
		all, err := jsonl.Load[ServerSnapshot](path)
		if err != nil {
			continue
		}
		var kept []ServerSnapshot
		for _, s := range all {
			if !s.Timestamp.Before(cutoff) {
				kept = append(kept, s)
			}
		}
		pruned := len(all) - len(kept)
		if pruned > 0 {
			_ = jsonl.Rewrite(path, kept)
			totalPruned += pruned
		}
	}
	return totalPruned, nil
}

// PruneBefore implements the Prunable interface for retention jobs.
func (r *ServerMetricsRepository) PruneBefore(cutoff time.Time) error {
	_, err := r.Prune(cutoff)
	return err
}

// SnapshotFromMetrics creates a ServerSnapshot from sshMetrics.
func SnapshotFromMetrics(serverID string, m *sshMetrics) ServerSnapshot {
	return ServerSnapshot{
		ServerID:           serverID,
		Timestamp:          time.Now().UTC(),
		CPUUsagePercent:    round2(m.CPUUsagePercent),
		MemoryTotalMB:      round2(m.MemoryTotalMB),
		MemoryUsedMB:       round2(m.MemoryUsedMB),
		MemoryUsagePercent: round2(m.MemoryUsagePercent),
		DiskTotalGB:        round2(m.DiskTotalGB),
		DiskUsedGB:         round2(m.DiskUsedGB),
		DiskUsagePercent:   round2(m.DiskUsagePercent),
		LoadAvg1:           round2(m.LoadAvg1),
		LoadAvg5:           round2(m.LoadAvg5),
		LoadAvg15:          round2(m.LoadAvg15),
		UptimeHours:        round2(m.UptimeSeconds / 3600),
		TopProcesses:       m.TopProcesses,
	}
}
