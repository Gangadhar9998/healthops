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
	}
}
