package monitoring

import (
	"context"
	"time"
)

// MySQLSampler collects raw MySQL metrics from a database target.
type MySQLSampler interface {
	Collect(ctx context.Context, check CheckConfig) (MySQLSample, error)
}

// MySQLSample holds raw counters and gauges from a single MySQL collection.
type MySQLSample struct {
	SampleID             string    `json:"sampleId" bson:"sampleId"`
	CheckID              string    `json:"checkId" bson:"checkId"`
	Timestamp            time.Time `json:"timestamp" bson:"timestamp"`
	Connections          int64     `json:"connections" bson:"connections"`
	MaxConnections       int64     `json:"maxConnections" bson:"maxConnections"`
	MaxUsedConnections   int64     `json:"maxUsedConnections" bson:"maxUsedConnections"`
	ThreadsRunning       int64     `json:"threadsRunning" bson:"threadsRunning"`
	ThreadsConnected     int64     `json:"threadsConnected" bson:"threadsConnected"`
	ThreadsCreated       int64     `json:"threadsCreated" bson:"threadsCreated"`
	AbortedConnects      int64     `json:"abortedConnects" bson:"abortedConnects"`
	AbortedClients       int64     `json:"abortedClients" bson:"abortedClients"`
	SlowQueries          int64     `json:"slowQueries" bson:"slowQueries"`
	Questions            int64     `json:"questions" bson:"questions"`
	QuestionsPerSec      float64   `json:"questionsPerSec" bson:"questionsPerSec"`
	UptimeSeconds        int64     `json:"uptimeSeconds" bson:"uptimeSeconds"`
	InnoDBRowLockWaits   int64     `json:"innodbRowLockWaits" bson:"innodbRowLockWaits"`
	InnoDBRowLockTime    int64     `json:"innodbRowLockTime" bson:"innodbRowLockTime"`
	CreatedTmpDiskTables int64     `json:"createdTmpDiskTables" bson:"createdTmpDiskTables"`
	CreatedTmpTables     int64     `json:"createdTmpTables" bson:"createdTmpTables"`
	ConnectionsRefused   int64     `json:"connectionsRefused" bson:"connectionsRefused"`
}

// MySQLDelta holds the computed delta between two consecutive samples.
type MySQLDelta struct {
	SampleID                string    `json:"sampleId" bson:"sampleId"`
	CheckID                 string    `json:"checkId" bson:"checkId"`
	IntervalSec             float64   `json:"intervalSec" bson:"intervalSec"`
	Timestamp               time.Time `json:"timestamp" bson:"timestamp"`
	AbortedConnectsDelta    int64     `json:"abortedConnectsDelta" bson:"abortedConnectsDelta"`
	AbortedConnectsPerSec   float64   `json:"abortedConnectsPerSec" bson:"abortedConnectsPerSec"`
	SlowQueriesDelta        int64     `json:"slowQueriesDelta" bson:"slowQueriesDelta"`
	SlowQueriesPerSec       float64   `json:"slowQueriesPerSec" bson:"slowQueriesPerSec"`
	QuestionsDelta          int64     `json:"questionsDelta" bson:"questionsDelta"`
	QuestionsPerSec         float64   `json:"questionsPerSec" bson:"questionsPerSec"`
	RowLockWaitsDelta       int64     `json:"rowLockWaitsDelta" bson:"rowLockWaitsDelta"`
	RowLockWaitsPerSec      float64   `json:"rowLockWaitsPerSec" bson:"rowLockWaitsPerSec"`
	TmpDiskTablesDelta      int64     `json:"tmpDiskTablesDelta" bson:"tmpDiskTablesDelta"`
	TmpDiskTablesPct        float64   `json:"tmpDiskTablesPct" bson:"tmpDiskTablesPct"`
	ThreadsCreatedDelta     int64     `json:"threadsCreatedDelta" bson:"threadsCreatedDelta"`
	ThreadsCreatedPerSec    float64   `json:"threadsCreatedPerSec" bson:"threadsCreatedPerSec"`
	ConnectionsRefusedDelta int64     `json:"connectionsRefusedDelta" bson:"connectionsRefusedDelta"`
}

// ComputeDelta calculates the delta between current and previous samples.
// Counter resets are handled with max(0, diff).
func ComputeDelta(current, previous MySQLSample) MySQLDelta {
	intervalSec := current.Timestamp.Sub(previous.Timestamp).Seconds()
	if intervalSec <= 0 {
		intervalSec = 1 // prevent division by zero
	}

	delta := MySQLDelta{
		SampleID:    current.SampleID,
		CheckID:     current.CheckID,
		IntervalSec: intervalSec,
		Timestamp:   current.Timestamp,
	}

	delta.AbortedConnectsDelta = counterDiff(current.AbortedConnects, previous.AbortedConnects)
	delta.AbortedConnectsPerSec = float64(delta.AbortedConnectsDelta) / intervalSec

	delta.SlowQueriesDelta = counterDiff(current.SlowQueries, previous.SlowQueries)
	delta.SlowQueriesPerSec = float64(delta.SlowQueriesDelta) / intervalSec

	delta.QuestionsDelta = counterDiff(current.Questions, previous.Questions)
	delta.QuestionsPerSec = float64(delta.QuestionsDelta) / intervalSec

	delta.RowLockWaitsDelta = counterDiff(current.InnoDBRowLockWaits, previous.InnoDBRowLockWaits)
	delta.RowLockWaitsPerSec = float64(delta.RowLockWaitsDelta) / intervalSec

	delta.TmpDiskTablesDelta = counterDiff(current.CreatedTmpDiskTables, previous.CreatedTmpDiskTables)
	tmpTablesDelta := counterDiff(current.CreatedTmpTables, previous.CreatedTmpTables)
	if tmpTablesDelta > 0 {
		delta.TmpDiskTablesPct = float64(delta.TmpDiskTablesDelta) / float64(tmpTablesDelta) * 100
	}

	delta.ThreadsCreatedDelta = counterDiff(current.ThreadsCreated, previous.ThreadsCreated)
	delta.ThreadsCreatedPerSec = float64(delta.ThreadsCreatedDelta) / intervalSec

	delta.ConnectionsRefusedDelta = counterDiff(current.ConnectionsRefused, previous.ConnectionsRefused)

	return delta
}

// counterDiff returns max(0, current-previous) to handle counter resets.
func counterDiff(current, previous int64) int64 {
	diff := current - previous
	if diff < 0 {
		return 0
	}
	return diff
}
