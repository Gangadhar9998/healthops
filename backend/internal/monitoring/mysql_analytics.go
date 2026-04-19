package monitoring

import (
	"encoding/csv"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// MySQLHealthSummary is a single-card summary of MySQL health.
type MySQLHealthSummary struct {
	CheckID               string            `json:"checkId"`
	Timestamp             time.Time         `json:"timestamp"`
	ConnectionUtilPct     float64           `json:"connectionUtilPct"`
	Connections           int64             `json:"connections"`
	MaxConnections        int64             `json:"maxConnections"`
	ThreadsRunning        int64             `json:"threadsRunning"`
	QueriesPerSec         float64           `json:"queriesPerSec"`
	SlowQueries           float64           `json:"slowQueries"`
	SlowQueriesPerSec     float64           `json:"slowQueriesPerSec"`
	RowLockWaitsPerSec    float64           `json:"rowLockWaitsPerSec"`
	TmpDiskTablesPct      float64           `json:"tmpDiskTablesPct"`
	AbortedConnectsPerSec float64           `json:"abortedConnectsPerSec"`
	UptimeSeconds         int64             `json:"uptimeSeconds"`
	Uptime                int64             `json:"uptime"`
	Status                string            `json:"status"` // "healthy", "warning", "critical"
	LastSampleAt          time.Time         `json:"lastSampleAt"`
	ProcessList           []MySQLProcess    `json:"processList,omitempty"`
	UserStats             []MySQLUserStat   `json:"userStats,omitempty"`
	HostStats             []MySQLHostStat   `json:"hostStats,omitempty"`
	TopQueries            []MySQLDigestStat `json:"topQueries,omitempty"`
	TotalSlowQueries      int64             `json:"totalSlowQueries"`
	AbortedConnects       int64             `json:"abortedConnects"`
	AbortedClients        int64             `json:"abortedClients"`
	MaxUsedConnections    int64             `json:"maxUsedConnections"`
	InnoDBRowLockWaits    int64             `json:"innodbRowLockWaits"`
	InnoDBRowLockTime     int64             `json:"innodbRowLockTime"`
	Questions             int64             `json:"questions"`
	// Additional performance stats
	SelectScan             int64   `json:"selectScan"`
	SelectFullJoin         int64   `json:"selectFullJoin"`
	SortMergePasses        int64   `json:"sortMergePasses"`
	TableLocksWaited       int64   `json:"tableLocksWaited"`
	TableLocksImmediate    int64   `json:"tableLocksImmediate"`
	BufferPoolHitRate      float64 `json:"bufferPoolHitRate"`
	OpenFiles              int64   `json:"openFiles"`
	OpenFilesLimit         int64   `json:"openFilesLimit"`
	OpenTables             int64   `json:"openTables"`
	TableOpenCache         int64   `json:"tableOpenCache"`
	OpenedTables           int64   `json:"openedTables"`
	ConnectionsRefused     int64   `json:"connectionsRefused"`
}

// MySQLTimeSeriesPoint for charting MySQL metrics over time.
type MySQLTimeSeriesPoint struct {
	Timestamp          time.Time `json:"timestamp"`
	ConnectionUtilPct  float64   `json:"connectionUtilPct,omitempty"`
	ThreadsRunning     int64     `json:"threadsRunning,omitempty"`
	QueriesPerSec      float64   `json:"queriesPerSec,omitempty"`
	SlowQueriesPerSec  float64   `json:"slowQueriesPerSec,omitempty"`
	RowLockWaitsPerSec float64   `json:"rowLockWaitsPerSec,omitempty"`
	TmpDiskTablesPct   float64   `json:"tmpDiskTablesPct,omitempty"`
	Connections        int64     `json:"connections,omitempty"`
	MaxConnections     int64     `json:"maxConnections,omitempty"`
}

// NotificationStats for notification analytics.
type NotificationStats struct {
	Total   int `json:"total"`
	Pending int `json:"pending"`
	Sent    int `json:"sent"`
	Failed  int `json:"failed"`
}

// AIQueueStats for AI queue analytics.
type AIQueueStats struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	Processing int `json:"processing"`
	Completed  int `json:"completed"`
	Failed     int `json:"failed"`
}

// ExportFormat type for data export.
type ExportFormat string

const (
	ExportCSV  ExportFormat = "csv"
	ExportJSON ExportFormat = "json"
)

// AllNotifications returns a copy of all notification events.
func (o *FileNotificationOutbox) AllNotifications() []NotificationEvent {
	o.mu.RLock()
	defer o.mu.RUnlock()

	out := make([]NotificationEvent, len(o.data))
	copy(out, o.data)
	return out
}

// AllItems returns a copy of all AI queue items.
func (q *FileAIQueue) AllItems() []AIQueueItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	out := make([]AIQueueItem, len(q.queue))
	copy(out, q.queue)
	return out
}

// RegisterAnalyticsRoutes registers analytics and export routes on the given mux.
func (h *MySQLAPIHandler) RegisterAnalyticsRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/mysql/health", h.handleMySQLHealthSummary)
	mux.HandleFunc("/api/v1/mysql/timeseries", h.handleMySQLTimeSeries)
	mux.HandleFunc("/api/v1/notifications/stats", h.handleNotificationStats)
	mux.HandleFunc("/api/v1/ai/queue/stats", h.handleAIQueueStats)
	mux.HandleFunc("/api/v1/export/mysql/samples", h.handleExportSamples)
}

// handleMySQLHealthSummary returns a single-card health summary for a MySQL check.
// GET /api/v1/mysql/health?checkId=
func (h *MySQLAPIHandler) handleMySQLHealthSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	checkID := strings.TrimSpace(r.URL.Query().Get("checkId"))
	if checkID == "" {
		checkID = h.firstMySQLCheckID()
	}
	if checkID == "" {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("checkId is required"))
		return
	}

	sample, err := h.mysqlRepo.LatestSample(checkID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}

	deltas, err := h.mysqlRepo.RecentDeltas(checkID, 1)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}

	var connUtilPct float64
	if sample.MaxConnections > 0 {
		connUtilPct = math.Round(float64(sample.Connections)/float64(sample.MaxConnections)*10000) / 100
	}

	status := "healthy"
	if connUtilPct > 90 {
		status = "critical"
	} else if connUtilPct > 70 {
		status = "warning"
	}

	summary := MySQLHealthSummary{
		CheckID:            checkID,
		Timestamp:          sample.Timestamp,
		ConnectionUtilPct:  connUtilPct,
		Connections:        sample.Connections,
		MaxConnections:     sample.MaxConnections,
		ThreadsRunning:     sample.ThreadsRunning,
		QueriesPerSec:      sample.QuestionsPerSec,
		UptimeSeconds:      sample.UptimeSeconds,
		Uptime:             sample.UptimeSeconds,
		Status:             status,
		LastSampleAt:       sample.Timestamp,
		ProcessList:        sample.ProcessList,
		UserStats:          sample.UserStats,
		HostStats:          sample.HostStats,
		TopQueries:         sample.TopQueries,
		TotalSlowQueries:   sample.SlowQueries,
		AbortedConnects:    sample.AbortedConnects,
		AbortedClients:     sample.AbortedClients,
		MaxUsedConnections: sample.MaxUsedConnections,
		InnoDBRowLockWaits: sample.InnoDBRowLockWaits,
		InnoDBRowLockTime:  sample.InnoDBRowLockTime,
		Questions:          sample.Questions,
		// Additional performance stats
		SelectScan:          sample.SelectScan,
		SelectFullJoin:      sample.SelectFullJoin,
		SortMergePasses:     sample.SortMergePasses,
		TableLocksWaited:    sample.TableLocksWaited,
		TableLocksImmediate: sample.TableLocksImmediate,
		OpenFiles:           sample.OpenFiles,
		OpenFilesLimit:      sample.OpenFilesLimit,
		OpenTables:          sample.OpenTables,
		TableOpenCache:      sample.TableOpenCache,
		OpenedTables:        sample.OpenedTables,
		ConnectionsRefused:  sample.ConnectionsRefused,
	}

	// Buffer pool hit rate
	if sample.BufferPoolReadRequests > 0 {
		summary.BufferPoolHitRate = math.Round((1.0-float64(sample.BufferPoolReads)/float64(sample.BufferPoolReadRequests))*10000) / 100
	}

	if len(deltas) > 0 {
		d := deltas[0]
		summary.SlowQueriesPerSec = d.SlowQueriesPerSec
		summary.SlowQueries = d.SlowQueriesPerSec
		summary.RowLockWaitsPerSec = d.RowLockWaitsPerSec
		summary.TmpDiskTablesPct = d.TmpDiskTablesPct
		summary.AbortedConnectsPerSec = d.AbortedConnectsPerSec
	}

	writeAPIResponse(w, http.StatusOK, NewAPIResponse(summary))
}

// handleMySQLTimeSeries returns time-series data for charting.
// GET /api/v1/mysql/timeseries?checkId=&metric=connections|qps|slow_queries|threads|locks&limit=100
func (h *MySQLAPIHandler) handleMySQLTimeSeries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	checkID := strings.TrimSpace(r.URL.Query().Get("checkId"))
	if checkID == "" {
		checkID = h.firstMySQLCheckID()
	}
	if checkID == "" {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("checkId is required"))
		return
	}

	metric := strings.TrimSpace(r.URL.Query().Get("metric"))
	if metric == "" {
		metric = "all"
	}
	limit := queryInt(r, "limit", 100)

	samples, err := h.mysqlRepo.RecentSamples(checkID, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}

	// Build delta lookup by sampleID for metrics that come from deltas.
	deltaMap := make(map[string]MySQLDelta)
	if metric == "all" || metric == "qps" || metric == "slow_queries" || metric == "locks" {
		deltas, err := h.mysqlRepo.RecentDeltas(checkID, limit)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, err)
			return
		}
		for _, d := range deltas {
			deltaMap[d.SampleID] = d
		}
	}

	points := make([]MySQLTimeSeriesPoint, 0, len(samples))
	for _, s := range samples {
		p := MySQLTimeSeriesPoint{Timestamp: s.Timestamp}
		d, hasDelta := deltaMap[s.SampleID]

		switch metric {
		case "connections":
			if s.MaxConnections > 0 {
				p.ConnectionUtilPct = math.Round(float64(s.Connections)/float64(s.MaxConnections)*10000) / 100
			}
			p.Connections = s.Connections
			p.MaxConnections = s.MaxConnections
		case "qps":
			if hasDelta {
				p.QueriesPerSec = d.QuestionsPerSec
			} else {
				p.QueriesPerSec = s.QuestionsPerSec
			}
		case "slow_queries":
			if hasDelta {
				p.SlowQueriesPerSec = d.SlowQueriesPerSec
			}
		case "threads":
			p.ThreadsRunning = s.ThreadsRunning
		case "locks":
			if hasDelta {
				p.RowLockWaitsPerSec = d.RowLockWaitsPerSec
			}
		default: // "all"
			if s.MaxConnections > 0 {
				p.ConnectionUtilPct = math.Round(float64(s.Connections)/float64(s.MaxConnections)*10000) / 100
			}
			p.Connections = s.Connections
			p.MaxConnections = s.MaxConnections
			p.ThreadsRunning = s.ThreadsRunning
			if hasDelta {
				p.QueriesPerSec = d.QuestionsPerSec
				p.SlowQueriesPerSec = d.SlowQueriesPerSec
				p.RowLockWaitsPerSec = d.RowLockWaitsPerSec
				p.TmpDiskTablesPct = d.TmpDiskTablesPct
			} else {
				p.QueriesPerSec = s.QuestionsPerSec
			}
		}

		points = append(points, p)
	}

	// Sort by timestamp ascending.
	sort.Slice(points, func(i, j int) bool {
		return points[i].Timestamp.Before(points[j].Timestamp)
	})

	writeAPIResponse(w, http.StatusOK, NewAPIResponse(points))
}

// handleNotificationStats returns aggregate notification statistics.
// GET /api/v1/notifications/stats
func (h *MySQLAPIHandler) handleNotificationStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	all := h.outbox.(*FileNotificationOutbox).AllNotifications()

	stats := NotificationStats{Total: len(all)}
	for _, evt := range all {
		switch evt.Status {
		case "pending":
			stats.Pending++
		case "sent":
			stats.Sent++
		case "failed":
			stats.Failed++
		}
	}

	writeAPIResponse(w, http.StatusOK, NewAPIResponse(stats))
}

// handleAIQueueStats returns aggregate AI queue statistics.
// GET /api/v1/ai/queue/stats
func (h *MySQLAPIHandler) handleAIQueueStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	all := h.aiQueue.AllItems()

	stats := AIQueueStats{Total: len(all)}
	for _, item := range all {
		switch item.Status {
		case "pending":
			stats.Pending++
		case "processing":
			stats.Processing++
		case "completed":
			stats.Completed++
		case "failed":
			stats.Failed++
		}
	}

	writeAPIResponse(w, http.StatusOK, NewAPIResponse(stats))
}

// handleExportSamples exports MySQL samples as CSV or JSON.
// GET /api/v1/export/mysql/samples?checkId=&limit=&format=csv|json
func (h *MySQLAPIHandler) handleExportSamples(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	checkID := strings.TrimSpace(r.URL.Query().Get("checkId"))
	if checkID == "" {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("checkId is required"))
		return
	}

	limit := queryInt(r, "limit", 100)
	format := ExportFormat(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = ExportJSON
	}

	samples, err := h.mysqlRepo.RecentSamples(checkID, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}

	// Sort ascending by timestamp for export.
	sort.Slice(samples, func(i, j int) bool {
		return samples[i].Timestamp.Before(samples[j].Timestamp)
	})

	if format == ExportCSV {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=mysql_samples_%s.csv", checkID))

		cw := csv.NewWriter(w)
		_ = cw.Write([]string{
			"sampleId", "checkId", "timestamp", "connections", "maxConnections",
			"threadsRunning", "threadsConnected", "slowQueries", "questionsPerSec", "uptimeSeconds",
		})
		for _, s := range samples {
			_ = cw.Write([]string{
				s.SampleID,
				s.CheckID,
				s.Timestamp.UTC().Format(time.RFC3339),
				strconv.FormatInt(s.Connections, 10),
				strconv.FormatInt(s.MaxConnections, 10),
				strconv.FormatInt(s.ThreadsRunning, 10),
				strconv.FormatInt(s.ThreadsConnected, 10),
				strconv.FormatInt(s.SlowQueries, 10),
				strconv.FormatFloat(s.QuestionsPerSec, 'f', 2, 64),
				strconv.FormatInt(s.UptimeSeconds, 10),
			})
		}
		cw.Flush()
		return
	}

	writeAPIResponse(w, http.StatusOK, NewAPIResponse(samples))
}

// handleExportIncidents returns an http.HandlerFunc that exports incidents as CSV or JSON.
// GET /api/v1/export/incidents?format=csv|json
func handleExportIncidents(incidentRepo IncidentRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		format := ExportFormat(strings.TrimSpace(r.URL.Query().Get("format")))
		if format == "" {
			format = ExportJSON
		}

		incidents, err := incidentRepo.ListIncidents()
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, err)
			return
		}

		if format == ExportCSV {
			w.Header().Set("Content-Type", "text/csv")
			w.Header().Set("Content-Disposition", "attachment; filename=incidents.csv")

			cw := csv.NewWriter(w)
			_ = cw.Write([]string{
				"id", "checkId", "checkName", "type", "status", "severity", "message", "startedAt", "resolvedAt",
			})
			for _, inc := range incidents {
				resolvedAt := ""
				if inc.ResolvedAt != nil {
					resolvedAt = inc.ResolvedAt.UTC().Format(time.RFC3339)
				}
				_ = cw.Write([]string{
					inc.ID,
					inc.CheckID,
					inc.CheckName,
					inc.Type,
					inc.Status,
					inc.Severity,
					inc.Message,
					inc.StartedAt.UTC().Format(time.RFC3339),
					resolvedAt,
				})
			}
			cw.Flush()
			return
		}

		writeAPIResponse(w, http.StatusOK, NewAPIResponse(incidents))
	}
}

// handleExportResults returns an http.HandlerFunc that exports check results as CSV or JSON.
// GET /api/v1/export/results?checkId=&days=&format=csv|json
func handleExportResults(store Store, retentionDays int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		format := ExportFormat(strings.TrimSpace(r.URL.Query().Get("format")))
		if format == "" {
			format = ExportJSON
		}

		checkID := strings.TrimSpace(r.URL.Query().Get("checkId"))
		days := retentionDays
		if d := r.URL.Query().Get("days"); d != "" {
			if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
				days = parsed
			}
		}

		snap := store.Snapshot()
		cutoff := time.Now().UTC().AddDate(0, 0, -days)

		var results []CheckResult
		for _, res := range snap.Results {
			if !res.StartedAt.Before(cutoff) {
				if checkID == "" || res.CheckID == checkID {
					results = append(results, res)
				}
			}
		}

		// Sort ascending by startedAt.
		sort.Slice(results, func(i, j int) bool {
			return results[i].StartedAt.Before(results[j].StartedAt)
		})

		if format == ExportCSV {
			w.Header().Set("Content-Type", "text/csv")
			w.Header().Set("Content-Disposition", "attachment; filename=results.csv")

			cw := csv.NewWriter(w)
			_ = cw.Write([]string{
				"id", "checkId", "name", "type", "server", "application",
				"status", "healthy", "durationMs", "startedAt", "finishedAt", "message",
			})
			for _, res := range results {
				_ = cw.Write([]string{
					res.ID,
					res.CheckID,
					res.Name,
					res.Type,
					res.Server,
					res.Application,
					res.Status,
					strconv.FormatBool(res.Healthy),
					strconv.FormatInt(res.DurationMs, 10),
					res.StartedAt.UTC().Format(time.RFC3339),
					res.FinishedAt.UTC().Format(time.RFC3339),
					res.Message,
				})
			}
			cw.Flush()
			return
		}

		writeAPIResponse(w, http.StatusOK, NewAPIResponse(results))
	}
}
