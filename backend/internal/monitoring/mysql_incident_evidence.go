package monitoring

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// IncidentSnapshotRepository persists evidence snapshots. Generic — not MySQL-specific.
type IncidentSnapshotRepository interface {
	SaveSnapshots(incidentID string, snaps []IncidentSnapshot) error
	GetSnapshots(incidentID string) ([]IncidentSnapshot, error)
	PruneBefore(cutoff time.Time) error
}

// FileSnapshotRepository implements IncidentSnapshotRepository with JSONL backing.
type FileSnapshotRepository struct {
	mu   sync.RWMutex
	path string
	data []IncidentSnapshot
}

// NewFileSnapshotRepository creates a file-backed snapshot repository.
func NewFileSnapshotRepository(path string) (*FileSnapshotRepository, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create snapshot dir: %w", err)
	}

	repo := &FileSnapshotRepository{path: path}
	var err error
	repo.data, err = loadJSONLFile[IncidentSnapshot](path)
	if err != nil {
		return nil, fmt.Errorf("load snapshots: %w", err)
	}
	return repo, nil
}

func (r *FileSnapshotRepository) SaveSnapshots(incidentID string, snaps []IncidentSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, snap := range snaps {
		snap.IncidentID = incidentID
		r.data = append(r.data, snap)
		if err := appendJSONLFile(r.path, snap); err != nil {
			return fmt.Errorf("append snapshot: %w", err)
		}
	}
	return nil
}

func (r *FileSnapshotRepository) GetSnapshots(incidentID string) ([]IncidentSnapshot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []IncidentSnapshot
	for _, s := range r.data {
		if s.IncidentID == incidentID {
			result = append(result, s)
		}
	}
	return result, nil
}

func (r *FileSnapshotRepository) PruneBefore(cutoff time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	pruned := r.data[:0]
	for _, s := range r.data {
		if !s.Timestamp.Before(cutoff) {
			pruned = append(pruned, s)
		}
	}
	r.data = pruned
	return rewriteJSONLFile(r.path, r.data)
}

// MySQLEvidenceCollector captures evidence snapshots from MySQL on incident open.
type MySQLEvidenceCollector struct {
	mysqlRepo MySQLMetricsRepository
}

// NewMySQLEvidenceCollector creates a new evidence collector.
func NewMySQLEvidenceCollector(mysqlRepo MySQLMetricsRepository) *MySQLEvidenceCollector {
	return &MySQLEvidenceCollector{mysqlRepo: mysqlRepo}
}

// CaptureEvidence collects all 7 snapshot types. Failures are captured as error payloads.
func (c *MySQLEvidenceCollector) CaptureEvidence(ctx context.Context, incidentID, checkID string, db *sql.DB) []IncidentSnapshot {
	now := time.Now().UTC()
	var snaps []IncidentSnapshot

	// 1. Latest sample
	if sample, err := c.mysqlRepo.LatestSample(checkID); err == nil {
		snaps = append(snaps, makeSnapshot(incidentID, "latest_sample", now, sample))
	} else {
		snaps = append(snaps, makeErrorSnapshot(incidentID, "latest_sample", now, err))
	}

	// 2. Recent deltas
	if deltas, err := c.mysqlRepo.RecentDeltas(checkID, 20); err == nil {
		snaps = append(snaps, makeSnapshot(incidentID, "recent_deltas", now, deltas))
	} else {
		snaps = append(snaps, makeErrorSnapshot(incidentID, "recent_deltas", now, err))
	}

	// 3-7. Database queries (only if db is provided)
	if db != nil {
		snaps = append(snaps, c.captureDBEvidence(ctx, incidentID, db, now)...)
	}

	return snaps
}

func (c *MySQLEvidenceCollector) captureDBEvidence(ctx context.Context, incidentID string, db *sql.DB, now time.Time) []IncidentSnapshot {
	var snaps []IncidentSnapshot

	queries := []struct {
		snapType string
		query    string
	}{
		{"processlist", "SELECT * FROM information_schema.PROCESSLIST ORDER BY TIME DESC LIMIT 50"},
		{"statement_analysis", "SELECT DIGEST_TEXT, COUNT_STAR, SUM_TIMER_WAIT, AVG_TIMER_WAIT FROM performance_schema.events_statements_summary_by_digest ORDER BY SUM_TIMER_WAIT DESC LIMIT 20"},
		{"host_summary", "SELECT HOST, CURRENT_CONNECTIONS, TOTAL_CONNECTIONS FROM performance_schema.hosts WHERE HOST IS NOT NULL ORDER BY CURRENT_CONNECTIONS DESC LIMIT 20"},
		{"user_summary", "SELECT USER, CURRENT_CONNECTIONS, TOTAL_CONNECTIONS FROM performance_schema.users WHERE USER IS NOT NULL ORDER BY CURRENT_CONNECTIONS DESC LIMIT 20"},
		{"host_cache", "SELECT IP, HOST, HOST_VALIDATED, SUM_CONNECT_ERRORS FROM performance_schema.host_cache ORDER BY SUM_CONNECT_ERRORS DESC LIMIT 20"},
	}

	for _, q := range queries {
		result, err := queryToJSON(ctx, db, q.query)
		if err != nil {
			snaps = append(snaps, makeErrorSnapshot(incidentID, q.snapType, now, err))
		} else {
			snaps = append(snaps, IncidentSnapshot{
				IncidentID:   incidentID,
				SnapshotType: q.snapType,
				Timestamp:    now,
				PayloadJSON:  result,
			})
		}
	}

	return snaps
}

func makeSnapshot(incidentID, snapType string, ts time.Time, payload interface{}) IncidentSnapshot {
	data, _ := json.Marshal(payload)
	return IncidentSnapshot{
		IncidentID:   incidentID,
		SnapshotType: snapType,
		Timestamp:    ts,
		PayloadJSON:  string(data),
	}
}

func makeErrorSnapshot(incidentID, snapType string, ts time.Time, err error) IncidentSnapshot {
	errPayload := map[string]string{"error": err.Error()}
	data, _ := json.Marshal(errPayload)
	return IncidentSnapshot{
		IncidentID:   incidentID,
		SnapshotType: snapType,
		Timestamp:    ts,
		PayloadJSON:  string(data),
	}
}

func queryToJSON(ctx context.Context, db *sql.DB, query string) (string, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}

	var results []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(cols))
		scanArgs := make([]interface{}, len(cols))
		for i := range values {
			scanArgs[i] = &values[i]
		}
		if err := rows.Scan(scanArgs...); err != nil {
			continue
		}

		row := make(map[string]interface{})
		for i, col := range cols {
			val := values[i]
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		results = append(results, row)
	}

	data, err := json.Marshal(results)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
