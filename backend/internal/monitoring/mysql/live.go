package mysql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"medics-health-check/backend/internal/monitoring"
)

// MySQLLiveSnapshot is a lightweight real-time snapshot streamed via SSE.
type MySQLLiveSnapshot struct {
	Timestamp         time.Time                 `json:"timestamp"`
	Connections       int64                     `json:"connections"`
	MaxConnections    int64                     `json:"maxConnections"`
	ConnectionUtilPct float64                   `json:"connectionUtilPct"`
	ThreadsRunning    int64                     `json:"threadsRunning"`
	ThreadsConnected  int64                     `json:"threadsConnected"`
	QueriesPerSec     float64                   `json:"queriesPerSec"`
	SlowQueries       int64                     `json:"slowQueries"`
	UptimeSeconds     int64                     `json:"uptimeSeconds"`
	ProcessList       []monitoring.MySQLProcess `json:"processList"`
	LongRunning       []monitoring.MySQLProcess `json:"longRunning"`
	ActiveQueries     int                       `json:"activeQueries"`
	LongRunningCount  int                       `json:"longRunningCount"`
	// Extended fields for full real-time coverage
	Status             string                     `json:"status"` // "healthy", "warning", "critical"
	AbortedConnects    int64                      `json:"abortedConnects"`
	AbortedClients     int64                      `json:"abortedClients"`
	ConnectionsRefused int64                      `json:"connectionsRefused"`
	MaxUsedConnections int64                      `json:"maxUsedConnections"`
	InnoDBRowLockWaits int64                      `json:"innodbRowLockWaits"`
	TableLocksWaited   int64                      `json:"tableLocksWaited"`
	BufferPoolHitRate  float64                    `json:"bufferPoolHitRate"`
	UserStats          []monitoring.MySQLUserStat `json:"userStats,omitempty"`
	HostStats          []monitoring.MySQLHostStat `json:"hostStats,omitempty"`
}

// KillQueryRequest is the body for POST /api/v1/mysql/kill.
type KillQueryRequest struct {
	ProcessID int64  `json:"processId"`
	CheckID   string `json:"checkId"`
}

// KillQueryResponse is the response for POST /api/v1/mysql/kill.
type KillQueryResponse struct {
	ProcessID int64  `json:"processId"`
	Status    string `json:"status"`
}

// RegisterLiveRoutes registers the real-time MySQL endpoints.
func (h *MySQLAPIHandler) RegisterLiveRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/mysql/live", h.handleMySQLLiveSSE)
	mux.HandleFunc("/api/v1/mysql/kill", h.handleMySQLKillQuery)
}

// handleMySQLLiveSSE streams real-time MySQL process list and key metrics via SSE.
// GET /api/v1/mysql/live?checkId=&interval=3
func (h *MySQLAPIHandler) handleMySQLLiveSSE(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		monitoring.WriteAPIError(w, http.StatusInternalServerError, fmt.Errorf("streaming not supported"))
		return
	}

	checkID := strings.TrimSpace(r.URL.Query().Get("checkId"))
	if checkID == "" {
		checkID = h.firstMySQLCheckID()
	}
	if checkID == "" {
		monitoring.WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("no mysql check configured"))
		return
	}

	intervalSec := monitoring.QueryInt(r, "interval", 3)
	if intervalSec < 2 {
		intervalSec = 2 // min 2 seconds to avoid overload
	}
	if intervalSec > 30 {
		intervalSec = 30
	}

	check := h.findMySQLCheck(checkID)
	if check == nil || check.MySQL == nil {
		monitoring.WriteAPIError(w, http.StatusNotFound, fmt.Errorf("mysql check %q not found", checkID))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // nginx proxy support

	ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
	defer ticker.Stop()

	// Send initial snapshot immediately
	h.sendLiveSnapshot(r.Context(), w, flusher, check)

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			h.sendLiveSnapshot(r.Context(), w, flusher, check)
		}
	}
}

func (h *MySQLAPIHandler) sendLiveSnapshot(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, check *monitoring.CheckConfig) {
	snapshot, err := h.collectLiveSnapshot(ctx, check)
	if err != nil {
		errData, _ := json.Marshal(map[string]string{"error": err.Error()})
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", errData)
		flusher.Flush()
		return
	}

	data, err := json.Marshal(snapshot)
	if err != nil {
		return
	}

	fmt.Fprintf(w, "event: mysql_live\ndata: %s\n\n", data)
	flusher.Flush()
}

func (h *MySQLAPIHandler) collectLiveSnapshot(ctx context.Context, check *monitoring.CheckConfig) (*MySQLLiveSnapshot, error) {
	dsn, err := check.MySQL.BuildDSN()
	if err != nil {
		return nil, fmt.Errorf("build dsn: %w", err)
	}

	connectTimeout := 3 * time.Second
	if check.MySQL.ConnectTimeoutSeconds > 0 {
		connectTimeout = time.Duration(check.MySQL.ConnectTimeoutSeconds) * time.Second
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("open connection: %w", err)
	}
	defer db.Close()
	db.SetConnMaxLifetime(connectTimeout)
	db.SetMaxOpenConns(1)

	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(queryCtx); err != nil {
		return nil, fmt.Errorf("ping failed: %w", err)
	}

	snap := &MySQLLiveSnapshot{
		Timestamp: time.Now().UTC(),
	}

	// Collect key status vars
	rows, err := db.QueryContext(queryCtx, "SHOW GLOBAL STATUS")
	if err != nil {
		return nil, fmt.Errorf("global status: %w", err)
	}
	defer rows.Close()
	var bufferPoolReads, bufferPoolReadRequests int64
	for rows.Next() {
		var name string
		var value sql.NullString
		if err := rows.Scan(&name, &value); err != nil || !value.Valid {
			continue
		}
		var v int64
		if _, err := fmt.Sscanf(value.String, "%d", &v); err != nil {
			continue
		}
		switch name {
		case "Threads_connected":
			snap.Connections = v
			snap.ThreadsConnected = v
		case "Threads_running":
			snap.ThreadsRunning = v
		case "Slow_queries":
			snap.SlowQueries = v
		case "Questions":
			snap.QueriesPerSec = float64(v)
		case "Uptime":
			snap.UptimeSeconds = v
		case "Aborted_connects":
			snap.AbortedConnects = v
		case "Aborted_clients":
			snap.AbortedClients = v
		case "Connection_errors_max_connections":
			snap.ConnectionsRefused = v
		case "Max_used_connections":
			snap.MaxUsedConnections = v
		case "Innodb_row_lock_waits":
			snap.InnoDBRowLockWaits = v
		case "Table_locks_waited":
			snap.TableLocksWaited = v
		case "Innodb_buffer_pool_reads":
			bufferPoolReads = v
		case "Innodb_buffer_pool_read_requests":
			bufferPoolReadRequests = v
		}
	}
	rows.Close()

	// Compute QPS
	if snap.UptimeSeconds > 0 {
		snap.QueriesPerSec = snap.QueriesPerSec / float64(snap.UptimeSeconds)
	}

	// Buffer pool hit rate
	if bufferPoolReadRequests > 0 {
		snap.BufferPoolHitRate = (1 - float64(bufferPoolReads)/float64(bufferPoolReadRequests)) * 100
	}

	// Get max_connections
	var varName, varVal sql.NullString
	row := db.QueryRowContext(queryCtx, "SHOW GLOBAL VARIABLES LIKE 'max_connections'")
	if err := row.Scan(&varName, &varVal); err == nil && varVal.Valid {
		if _, err := fmt.Sscanf(varVal.String, "%d", &snap.MaxConnections); err != nil {
			snap.MaxConnections = 0
		}
	}

	// Connection utilization
	if snap.MaxConnections > 0 {
		snap.ConnectionUtilPct = float64(snap.Connections) / float64(snap.MaxConnections) * 100
	}

	// Collect process list
	procRows, err := db.QueryContext(queryCtx, "SHOW FULL PROCESSLIST")
	if err == nil {
		defer procRows.Close()
		for procRows.Next() {
			var (
				id             int64
				user, host     string
				dbName, info   sql.NullString
				command, state sql.NullString
				timeVal        int64
			)
			if err := procRows.Scan(&id, &user, &host, &dbName, &command, &timeVal, &state, &info); err != nil {
				continue
			}
			p := monitoring.MySQLProcess{
				ID:      id,
				User:    user,
				Host:    host,
				DB:      dbName.String,
				Command: command.String,
				Time:    timeVal,
				State:   state.String,
				Info:    info.String,
			}
			snap.ProcessList = append(snap.ProcessList, p)

			// Track active queries
			if p.Command != "Sleep" && p.Command != "Daemon" {
				snap.ActiveQueries++
			}
			// Long running: queries > 5 seconds
			if p.Time > 5 && p.Command != "Daemon" && p.Command != "Sleep" {
				snap.LongRunning = append(snap.LongRunning, p)
			}
		}
		procRows.Close()
	}

	snap.LongRunningCount = len(snap.LongRunning)

	// Collect user stats from performance_schema
	userRows, err := db.QueryContext(queryCtx, "SELECT USER, CURRENT_CONNECTIONS, TOTAL_CONNECTIONS FROM performance_schema.users WHERE USER IS NOT NULL ORDER BY CURRENT_CONNECTIONS DESC LIMIT 10")
	if err == nil {
		defer userRows.Close()
		for userRows.Next() {
			var u monitoring.MySQLUserStat
			if err := userRows.Scan(&u.User, &u.CurrentConnections, &u.TotalConnections); err != nil {
				continue
			}
			snap.UserStats = append(snap.UserStats, u)
		}
		userRows.Close()
	}

	// Collect host stats from performance_schema
	hostRows, err := db.QueryContext(queryCtx, "SELECT HOST, CURRENT_CONNECTIONS, TOTAL_CONNECTIONS FROM performance_schema.hosts WHERE HOST IS NOT NULL ORDER BY CURRENT_CONNECTIONS DESC LIMIT 10")
	if err == nil {
		defer hostRows.Close()
		for hostRows.Next() {
			var h monitoring.MySQLHostStat
			if err := hostRows.Scan(&h.Host, &h.CurrentConnections, &h.TotalConnections); err != nil {
				continue
			}
			snap.HostStats = append(snap.HostStats, h)
		}
		hostRows.Close()
	}

	// Derive status
	snap.Status = "healthy"
	if snap.ConnectionUtilPct > 80 || snap.LongRunningCount > 0 || snap.InnoDBRowLockWaits > 100 {
		snap.Status = "warning"
	}
	if snap.ConnectionUtilPct > 95 || snap.LongRunningCount > 5 {
		snap.Status = "critical"
	}

	return snap, nil
}

// handleMySQLKillQuery kills a MySQL process/query.
// POST /api/v1/mysql/kill
func (h *MySQLAPIHandler) handleMySQLKillQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if !monitoring.IsRequestAuthorized(h.cfg.Auth, r) {
		monitoring.RequestAuth(w)
		return
	}

	var req KillQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		monitoring.WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}

	if req.ProcessID <= 0 {
		monitoring.WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("processId is required and must be positive"))
		return
	}

	checkID := req.CheckID
	if checkID == "" {
		checkID = h.firstMySQLCheckID()
	}

	check := h.findMySQLCheck(checkID)
	if check == nil || check.MySQL == nil {
		monitoring.WriteAPIError(w, http.StatusNotFound, fmt.Errorf("mysql check %q not found", checkID))
		return
	}

	dsn, err := check.MySQL.BuildDSN()
	if err != nil {
		monitoring.WriteAPIError(w, http.StatusInternalServerError, fmt.Errorf("build dsn: %w", err))
		return
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		monitoring.WriteAPIError(w, http.StatusInternalServerError, fmt.Errorf("open connection: %w", err))
		return
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	// Use KILL QUERY to only kill the running query, not the connection.
	// This is safer — the connection stays alive, the query is cancelled.
	// Using parameterized int to prevent SQL injection.
	_, err = db.ExecContext(ctx, fmt.Sprintf("KILL QUERY %d", req.ProcessID))
	if err != nil {
		// If the process doesn't exist, MySQL returns error 1094
		if strings.Contains(err.Error(), "Unknown thread id") {
			monitoring.WriteAPIError(w, http.StatusNotFound, fmt.Errorf("process %d not found (may have already completed)", req.ProcessID))
			return
		}
		monitoring.WriteAPIError(w, http.StatusInternalServerError, fmt.Errorf("kill query: %w", err))
		return
	}

	if h.auditLogger != nil {
		actor := monitoring.ExtractActorFromRequest(r, h.cfg)
		_ = h.auditLogger.Log("mysql.query.killed", actor, "mysql_process", fmt.Sprintf("%d", req.ProcessID), map[string]interface{}{
			"checkId":   checkID,
			"processId": req.ProcessID,
		})
	}

	monitoring.WriteAPIResponse(w, http.StatusOK, monitoring.NewAPIResponse(KillQueryResponse{
		ProcessID: req.ProcessID,
		Status:    "killed",
	}))
}

// findMySQLCheck finds a MySQL check config by ID.
func (h *MySQLAPIHandler) findMySQLCheck(checkID string) *monitoring.CheckConfig {
	if h.cfg == nil {
		return nil
	}
	for i := range h.cfg.Checks {
		if h.cfg.Checks[i].ID == checkID && h.cfg.Checks[i].Type == "mysql" {
			return &h.cfg.Checks[i]
		}
	}
	return nil
}
