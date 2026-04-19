package monitoring

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// LiveMySQLSampler collects metrics from a real MySQL database.
type LiveMySQLSampler struct{}

// NewLiveMySQLSampler creates a new live MySQL sampler.
func NewLiveMySQLSampler() *LiveMySQLSampler {
	return &LiveMySQLSampler{}
}

// Collect gathers MySQL status variables and computes a sample.
// SECURITY: DSN is resolved from the environment variable named in check.MySQL.DSNEnv.
// The DSN value is never logged or returned in responses.
func (s *LiveMySQLSampler) Collect(ctx context.Context, check CheckConfig) (MySQLSample, error) {
	if check.MySQL == nil {
		return MySQLSample{}, fmt.Errorf("mysql config block is required")
	}

	dsn := os.Getenv(check.MySQL.DSNEnv)
	if dsn == "" {
		return MySQLSample{}, fmt.Errorf("environment variable %q is not set (required for mysql check %q)", check.MySQL.DSNEnv, check.ID)
	}

	connectTimeout := time.Duration(check.MySQL.ConnectTimeoutSeconds) * time.Second
	if connectTimeout <= 0 {
		connectTimeout = 3 * time.Second
	}

	queryTimeout := time.Duration(check.MySQL.QueryTimeoutSeconds) * time.Second
	if queryTimeout <= 0 {
		queryTimeout = 5 * time.Second
	}

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return MySQLSample{}, fmt.Errorf("open mysql connection: %w", err)
	}
	defer db.Close()

	db.SetConnMaxLifetime(connectTimeout)
	db.SetMaxOpenConns(1)

	connCtx, connCancel := context.WithTimeout(ctx, connectTimeout)
	defer connCancel()
	if err := db.PingContext(connCtx); err != nil {
		return MySQLSample{}, fmt.Errorf("mysql ping failed: %w", err)
	}

	queryCtx, queryCancel := context.WithTimeout(ctx, queryTimeout)
	defer queryCancel()

	sample := MySQLSample{
		SampleID:  fmt.Sprintf("%s-%d", check.ID, time.Now().UnixNano()),
		CheckID:   check.ID,
		Timestamp: time.Now().UTC(),
	}

	if err := s.collectGlobalStatus(queryCtx, db, &sample); err != nil {
		return MySQLSample{}, fmt.Errorf("collect global status: %w", err)
	}

	if err := s.collectGlobalVariables(queryCtx, db, &sample); err != nil {
		return MySQLSample{}, fmt.Errorf("collect global variables: %w", err)
	}

	if err := s.collectProcessList(queryCtx, db, &sample); err != nil {
		// Non-fatal: process list is supplementary data
		sample.ProcessList = nil
	}

	// Collect per-user connection stats from performance_schema
	if err := s.collectUserStats(queryCtx, db, &sample, check); err != nil {
		sample.UserStats = nil
	}

	// Collect per-host connection stats from performance_schema
	if err := s.collectHostStats(queryCtx, db, &sample, check); err != nil {
		sample.HostStats = nil
	}

	// Collect top query digests from performance_schema
	if err := s.collectTopQueries(queryCtx, db, &sample, check); err != nil {
		sample.TopQueries = nil
	}

	return sample, nil
}

func (s *LiveMySQLSampler) collectGlobalStatus(ctx context.Context, db *sql.DB, sample *MySQLSample) error {
	rows, err := db.QueryContext(ctx, "SHOW GLOBAL STATUS")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var value sql.NullString
		if err := rows.Scan(&name, &value); err != nil {
			continue
		}
		if !value.Valid {
			continue
		}
		s.applyStatusVar(name, value.String, sample)
	}
	return rows.Err()
}

func (s *LiveMySQLSampler) collectGlobalVariables(ctx context.Context, db *sql.DB, sample *MySQLSample) error {
	rows, err := db.QueryContext(ctx, "SHOW GLOBAL VARIABLES")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		var value sql.NullString
		if err := rows.Scan(&name, &value); err != nil {
			continue
		}
		if !value.Valid {
			continue
		}
		s.applyVariableVar(name, value.String, sample)
	}
	return rows.Err()
}

func (s *LiveMySQLSampler) applyStatusVar(name, value string, sample *MySQLSample) {
	var v int64
	if _, err := fmt.Sscanf(value, "%d", &v); err != nil {
		return
	}

	switch name {
	case "Threads_connected":
		sample.Connections = v
		sample.ThreadsConnected = v
	case "Threads_running":
		sample.ThreadsRunning = v
	case "Threads_created":
		sample.ThreadsCreated = v
	case "Max_used_connections":
		sample.MaxUsedConnections = v
	case "Aborted_connects":
		sample.AbortedConnects = v
	case "Aborted_clients":
		sample.AbortedClients = v
	case "Slow_queries":
		sample.SlowQueries = v
	case "Questions":
		sample.Questions = v
	case "Uptime":
		sample.UptimeSeconds = v
		if v > 0 {
			sample.QuestionsPerSec = float64(sample.Questions) / float64(v)
		}
	case "Innodb_row_lock_waits":
		sample.InnoDBRowLockWaits = v
	case "Innodb_row_lock_time":
		sample.InnoDBRowLockTime = v
	case "Created_tmp_disk_tables":
		sample.CreatedTmpDiskTables = v
	case "Created_tmp_tables":
		sample.CreatedTmpTables = v
	case "Connection_errors_max_connections":
		sample.ConnectionsRefused = v
	case "Select_scan":
		sample.SelectScan = v
	case "Select_full_join":
		sample.SelectFullJoin = v
	case "Sort_merge_passes":
		sample.SortMergePasses = v
	case "Handler_read_rnd_next":
		sample.HandlerReadRndNext = v
	case "Innodb_buffer_pool_read_requests":
		sample.BufferPoolReadRequests = v
	case "Innodb_buffer_pool_reads":
		sample.BufferPoolReads = v
	case "Table_locks_waited":
		sample.TableLocksWaited = v
	case "Table_locks_immediate":
		sample.TableLocksImmediate = v
	case "Open_files":
		sample.OpenFiles = v
	case "Open_tables":
		sample.OpenTables = v
	case "Opened_tables":
		sample.OpenedTables = v
	}
}

func (s *LiveMySQLSampler) applyVariableVar(name, value string, sample *MySQLSample) {
	var v int64
	if _, err := fmt.Sscanf(value, "%d", &v); err != nil {
		return
	}

	switch name {
	case "max_connections":
		sample.MaxConnections = v
	case "open_files_limit":
		sample.OpenFilesLimit = v
	case "table_open_cache":
		sample.TableOpenCache = v
	}
}

func (s *LiveMySQLSampler) collectProcessList(ctx context.Context, db *sql.DB, sample *MySQLSample) error {
	rows, err := db.QueryContext(ctx, "SHOW FULL PROCESSLIST")
	if err != nil {
		return err
	}
	defer rows.Close()

	var processes []MySQLProcess
	for rows.Next() {
		var (
			id                 int64
			user, host         string
			dbName, info       sql.NullString
			command, state     sql.NullString
			timeVal            int64
		)
		if err := rows.Scan(&id, &user, &host, &dbName, &command, &timeVal, &state, &info); err != nil {
			continue
		}
		p := MySQLProcess{
			ID:      id,
			User:    user,
			Host:    host,
			DB:      dbName.String,
			Command: command.String,
			Time:    timeVal,
			State:   state.String,
			Info:    info.String,
		}
		processes = append(processes, p)
	}
	sample.ProcessList = processes
	return rows.Err()
}

func (s *LiveMySQLSampler) collectUserStats(ctx context.Context, db *sql.DB, sample *MySQLSample, check CheckConfig) error {
	limit := 20
	if check.MySQL != nil && check.MySQL.HostUserLimit > 0 {
		limit = check.MySQL.HostUserLimit
	}

	rows, err := db.QueryContext(ctx, fmt.Sprintf(
		"SELECT USER, CURRENT_CONNECTIONS, TOTAL_CONNECTIONS FROM performance_schema.users WHERE USER IS NOT NULL ORDER BY CURRENT_CONNECTIONS DESC LIMIT %d", limit))
	if err != nil {
		return err
	}
	defer rows.Close()

	var stats []MySQLUserStat
	for rows.Next() {
		var u MySQLUserStat
		if err := rows.Scan(&u.User, &u.CurrentConnections, &u.TotalConnections); err != nil {
			continue
		}
		stats = append(stats, u)
	}
	sample.UserStats = stats
	return rows.Err()
}

func (s *LiveMySQLSampler) collectHostStats(ctx context.Context, db *sql.DB, sample *MySQLSample, check CheckConfig) error {
	limit := 20
	if check.MySQL != nil && check.MySQL.HostUserLimit > 0 {
		limit = check.MySQL.HostUserLimit
	}

	rows, err := db.QueryContext(ctx, fmt.Sprintf(
		"SELECT HOST, CURRENT_CONNECTIONS, TOTAL_CONNECTIONS FROM performance_schema.hosts WHERE HOST IS NOT NULL ORDER BY CURRENT_CONNECTIONS DESC LIMIT %d", limit))
	if err != nil {
		return err
	}
	defer rows.Close()

	var stats []MySQLHostStat
	for rows.Next() {
		var h MySQLHostStat
		if err := rows.Scan(&h.Host, &h.CurrentConnections, &h.TotalConnections); err != nil {
			continue
		}
		stats = append(stats, h)
	}
	sample.HostStats = stats
	return rows.Err()
}

func (s *LiveMySQLSampler) collectTopQueries(ctx context.Context, db *sql.DB, sample *MySQLSample, check CheckConfig) error {
	limit := 20
	if check.MySQL != nil && check.MySQL.StatementLimit > 0 {
		limit = check.MySQL.StatementLimit
	}

	rows, err := db.QueryContext(ctx, fmt.Sprintf(
		`SELECT IFNULL(DIGEST_TEXT, ''), COUNT_STAR,
		 ROUND(SUM_TIMER_WAIT/1000000000000, 4),
		 ROUND(AVG_TIMER_WAIT/1000000000000, 4),
		 SUM_ROWS_SENT, SUM_ROWS_EXAMINED,
		 SUM_ERRORS, SUM_WARNINGS,
		 IFNULL(FIRST_SEEN, ''), IFNULL(LAST_SEEN, '')
		 FROM performance_schema.events_statements_summary_by_digest
		 WHERE DIGEST_TEXT IS NOT NULL
		 ORDER BY SUM_TIMER_WAIT DESC LIMIT %d`, limit))
	if err != nil {
		return err
	}
	defer rows.Close()

	var stats []MySQLDigestStat
	for rows.Next() {
		var d MySQLDigestStat
		if err := rows.Scan(&d.DigestText, &d.CountStar, &d.SumTimerWait, &d.AvgTimerWait, &d.SumRowsSent, &d.SumRowsExam, &d.SumErrors, &d.SumWarnings, &d.FirstSeen, &d.LastSeen); err != nil {
			continue
		}
		// Truncate long digest text for API responses
		if len(d.DigestText) > 200 {
			d.DigestText = d.DigestText[:200] + "..."
		}
		stats = append(stats, d)
	}
	sample.TopQueries = stats
	return rows.Err()
}
