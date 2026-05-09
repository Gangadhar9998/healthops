package monitoring

import "time"

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

type ServerMetricsStore interface {
	Save(snap ServerSnapshot) error
	GetSnapshots(serverID string, since, until time.Time) ([]ServerSnapshot, error)
	GetLatest(serverID string) (*ServerSnapshot, error)
	PruneBefore(cutoff time.Time) error
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
