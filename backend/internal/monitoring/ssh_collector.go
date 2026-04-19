package monitoring

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// SSHCheckConfig holds SSH-specific check configuration.
type SSHCheckConfig struct {
	Host        string   `json:"host" bson:"host"`
	Port        int      `json:"port,omitempty" bson:"port,omitempty"`               // default 22
	User        string   `json:"user" bson:"user"`
	KeyPath     string   `json:"keyPath,omitempty" bson:"keyPath,omitempty"`         // path to private key file
	KeyEnv      string   `json:"keyEnv,omitempty" bson:"keyEnv,omitempty"`           // env var with path to key file
	Password    string   `json:"password,omitempty" bson:"password,omitempty"`       // password for password auth
	PasswordEnv string   `json:"passwordEnv,omitempty" bson:"passwordEnv,omitempty"` // env var holding the password
	Metrics     []string `json:"metrics,omitempty" bson:"metrics,omitempty"`         // cpu, memory, disk, load (empty = all)
}

// sshMetrics holds the parsed metrics from a remote server.
type sshMetrics struct {
	CPUUsagePercent    float64
	MemoryTotalMB      float64
	MemoryUsedMB       float64
	MemoryUsagePercent float64
	DiskTotalGB        float64
	DiskUsedGB         float64
	DiskUsagePercent   float64
	LoadAvg1           float64
	LoadAvg5           float64
	LoadAvg15          float64
	UptimeSeconds      float64
	DiskReadIOPS       float64
	DiskWriteIOPS      float64
}

// collectSSHMetrics connects to a remote server via SSH and collects system metrics.
func collectSSHMetrics(cfg *SSHCheckConfig, timeout time.Duration) (*sshMetrics, error) {
	authMethods, err := buildSSHAuth(cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh auth: %w", err)
	}

	port := cfg.Port
	if port <= 0 {
		port = 22
	}

	sshConfig := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: use known_hosts in production
		Timeout:         timeout,
	}

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(port))
	client, err := ssh.Dial("tcp", addr, sshConfig)
	if err != nil {
		return nil, fmt.Errorf("ssh connect %s: %w", addr, err)
	}
	defer client.Close()

	// Run a single compound command to minimise round-trips.
	cmd := buildMetricsCommand(cfg.Metrics)
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	out, err := session.CombinedOutput(cmd)
	if err != nil {
		return nil, fmt.Errorf("ssh exec: %w\noutput: %s", err, string(out))
	}

	return parseMetricsOutput(string(out))
}

// buildSSHAuth builds the authentication methods: key-based, password-based, or both.
func buildSSHAuth(cfg *SSHCheckConfig) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	// Try key-based auth first
	if cfg.KeyPath != "" || cfg.KeyEnv != "" {
		signer, err := loadSSHKey(cfg)
		if err != nil {
			return nil, err
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}

	// Try password auth
	password := cfg.Password
	if password == "" && cfg.PasswordEnv != "" {
		password = os.Getenv(cfg.PasswordEnv)
	}
	if password != "" {
		methods = append(methods, ssh.Password(password))
	}

	if len(methods) == 0 {
		return nil, fmt.Errorf("no SSH auth configured: set keyPath/keyEnv or password/passwordEnv")
	}
	return methods, nil
}

// sshDialAndRun connects to a remote server via SSH and executes a command.
// Returns the combined stdout+stderr output. Used by process, command, and log checks
// that reference a remote server via serverId.
func sshDialAndRun(cfg *SSHCheckConfig, command string, timeout time.Duration) ([]byte, error) {
	authMethods, err := buildSSHAuth(cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh auth: %w", err)
	}

	port := cfg.Port
	if port <= 0 {
		port = 22
	}

	sshCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(port))
	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return nil, fmt.Errorf("ssh connect %s: %w", addr, err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("ssh session: %w", err)
	}
	defer session.Close()

	out, err := session.CombinedOutput(command)
	return out, err
}

// loadSSHKey loads the SSH private key from KeyPath or the file pointed to by KeyEnv.
func loadSSHKey(cfg *SSHCheckConfig) (ssh.Signer, error) {
	path := cfg.KeyPath
	if path == "" && cfg.KeyEnv != "" {
		path = os.Getenv(cfg.KeyEnv)
	}
	if path == "" {
		return nil, fmt.Errorf("no SSH key configured: set keyPath or keyEnv")
	}

	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read key %s: %w", path, err)
	}

	signer, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("parse key: %w", err)
	}
	return signer, nil
}

// buildMetricsCommand builds a single shell command that prints all requested sections.
func buildMetricsCommand(metrics []string) string {
	wantAll := len(metrics) == 0
	want := map[string]bool{}
	for _, m := range metrics {
		want[strings.ToLower(m)] = true
	}

	parts := []string{}

	if wantAll || want["cpu"] {
		// Read /proc/stat twice with a 1-second gap to compute CPU usage
		parts = append(parts, `echo "===CPU==="`,
			`cat /proc/stat | head -1`,
			`sleep 1`,
			`cat /proc/stat | head -1`)
	}
	if wantAll || want["memory"] {
		parts = append(parts, `echo "===MEM==="`, `free -b`)
	}
	if wantAll || want["disk"] {
		parts = append(parts, `echo "===DISK==="`, `df -B1 / | tail -1`)
	}
	if wantAll || want["load"] {
		parts = append(parts, `echo "===LOAD==="`, `cat /proc/loadavg`)
	}
	if wantAll || want["uptime"] {
		parts = append(parts, `echo "===UPTIME==="`, `cat /proc/uptime`)
	}
	if wantAll || want["iops"] {
		// diskstats: read twice with the 1s gap from CPU to compute delta
		parts = append(parts, `echo "===DISKIO==="`, `cat /proc/diskstats`)
	}

	return strings.Join(parts, " && ")
}

// parseMetricsOutput parses the compound command output into sshMetrics.
func parseMetricsOutput(raw string) (*sshMetrics, error) {
	m := &sshMetrics{}

	sections := splitSections(raw)

	if cpuLines, ok := sections["CPU"]; ok {
		m.CPUUsagePercent = parseCPU(cpuLines)
	}
	if memLines, ok := sections["MEM"]; ok {
		parseMem(memLines, m)
	}
	if diskLines, ok := sections["DISK"]; ok {
		parseDisk(diskLines, m)
	}
	if loadLines, ok := sections["LOAD"]; ok {
		parseLoad(loadLines, m)
	}
	if uptimeLines, ok := sections["UPTIME"]; ok {
		parseUptime(uptimeLines, m)
	}
	if ioLines, ok := sections["DISKIO"]; ok {
		parseDiskIO(ioLines, m)
	}

	return m, nil
}

// splitSections splits the raw output into named sections by ===NAME=== markers.
func splitSections(raw string) map[string][]string {
	sections := map[string][]string{}
	var current string
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "===") && strings.HasSuffix(line, "===") {
			current = strings.Trim(line, "= ")
			sections[current] = []string{}
			continue
		}
		if current != "" && line != "" {
			sections[current] = append(sections[current], line)
		}
	}
	return sections
}

// parseCPU computes CPU usage % from two /proc/stat snapshots.
// Line format: cpu  user nice system idle iowait irq softirq steal guest guest_nice
func parseCPU(lines []string) float64 {
	if len(lines) < 2 {
		return 0
	}
	parse := func(line string) (idle, total float64) {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return 0, 0
		}
		var vals []float64
		for _, f := range fields[1:] { // skip "cpu"
			v, _ := strconv.ParseFloat(f, 64)
			vals = append(vals, v)
		}
		for _, v := range vals {
			total += v
		}
		if len(vals) >= 4 {
			idle = vals[3] // idle is 4th field
		}
		return idle, total
	}

	idle1, total1 := parse(lines[0])
	idle2, total2 := parse(lines[1])

	totalDelta := total2 - total1
	idleDelta := idle2 - idle1
	if totalDelta <= 0 {
		return 0
	}
	return ((totalDelta - idleDelta) / totalDelta) * 100
}

// parseMem parses `free -b` output.
func parseMem(lines []string, m *sshMetrics) {
	for _, line := range lines {
		if strings.HasPrefix(line, "Mem:") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				total, _ := strconv.ParseFloat(fields[1], 64)
				used, _ := strconv.ParseFloat(fields[2], 64)
				m.MemoryTotalMB = total / (1024 * 1024)
				m.MemoryUsedMB = used / (1024 * 1024)
				if total > 0 {
					m.MemoryUsagePercent = (used / total) * 100
				}
			}
		}
	}
}

// parseDisk parses `df -B1 / | tail -1` output.
func parseDisk(lines []string, m *sshMetrics) {
	if len(lines) == 0 {
		return
	}
	fields := strings.Fields(lines[0])
	if len(fields) >= 5 {
		total, _ := strconv.ParseFloat(fields[1], 64)
		used, _ := strconv.ParseFloat(fields[2], 64)
		m.DiskTotalGB = total / (1024 * 1024 * 1024)
		m.DiskUsedGB = used / (1024 * 1024 * 1024)
		pctStr := strings.TrimSuffix(fields[4], "%")
		m.DiskUsagePercent, _ = strconv.ParseFloat(pctStr, 64)
	}
}

// parseLoad parses /proc/loadavg.
func parseLoad(lines []string, m *sshMetrics) {
	if len(lines) == 0 {
		return
	}
	fields := strings.Fields(lines[0])
	if len(fields) >= 3 {
		m.LoadAvg1, _ = strconv.ParseFloat(fields[0], 64)
		m.LoadAvg5, _ = strconv.ParseFloat(fields[1], 64)
		m.LoadAvg15, _ = strconv.ParseFloat(fields[2], 64)
	}
}

// parseUptime parses /proc/uptime (seconds since boot).
func parseUptime(lines []string, m *sshMetrics) {
	if len(lines) == 0 {
		return
	}
	fields := strings.Fields(lines[0])
	if len(fields) >= 1 {
		m.UptimeSeconds, _ = strconv.ParseFloat(fields[0], 64)
	}
}

// parseDiskIO parses /proc/diskstats for aggregate reads/writes.
// We sum reads (field 4) and writes (field 8) across all block devices (excluding partitions).
func parseDiskIO(lines []string, m *sshMetrics) {
	var totalReads, totalWrites float64
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 14 {
			continue
		}
		devName := fields[2]
		// Skip partitions (e.g., sda1, nvme0n1p1) — only count whole devices
		if isPartition(devName) {
			continue
		}
		reads, _ := strconv.ParseFloat(fields[3], 64)  // reads completed
		writes, _ := strconv.ParseFloat(fields[7], 64) // writes completed
		totalReads += reads
		totalWrites += writes
	}
	m.DiskReadIOPS = totalReads
	m.DiskWriteIOPS = totalWrites
}

// isPartition returns true if the device name looks like a partition.
func isPartition(name string) bool {
	// Common patterns: sda1, nvme0n1p1, vda1, xvda1
	if len(name) == 0 {
		return false
	}
	last := name[len(name)-1]
	if last >= '0' && last <= '9' {
		// Check if there's a 'p' before the number for nvme devices
		if strings.Contains(name, "nvme") && strings.Contains(name, "p") {
			return true
		}
		// For sd*, vd*, xvd* — if it ends in a digit after letters, it's a partition
		if strings.HasPrefix(name, "sd") || strings.HasPrefix(name, "vd") || strings.HasPrefix(name, "xvd") {
			// sda = disk, sda1 = partition
			// Check if the char before last digit is a letter
			for i := len(name) - 1; i >= 0; i-- {
				if name[i] < '0' || name[i] > '9' {
					if name[i] >= 'a' && name[i] <= 'z' {
						return true // e.g., sda1
					}
					break
				}
			}
		}
	}
	return false
}

// metricsToMap converts sshMetrics to a map for result.Metrics.
func (m *sshMetrics) toMap() map[string]float64 {
	r := map[string]float64{}
	if m.CPUUsagePercent > 0 {
		r["cpuUsagePercent"] = round2(m.CPUUsagePercent)
	}
	if m.MemoryTotalMB > 0 {
		r["memoryTotalMB"] = round2(m.MemoryTotalMB)
		r["memoryUsedMB"] = round2(m.MemoryUsedMB)
		r["memoryUsagePercent"] = round2(m.MemoryUsagePercent)
	}
	if m.DiskTotalGB > 0 {
		r["diskTotalGB"] = round2(m.DiskTotalGB)
		r["diskUsedGB"] = round2(m.DiskUsedGB)
		r["diskUsagePercent"] = round2(m.DiskUsagePercent)
	}
	if m.LoadAvg1 > 0 || m.LoadAvg5 > 0 {
		r["loadAvg1"] = round2(m.LoadAvg1)
		r["loadAvg5"] = round2(m.LoadAvg5)
		r["loadAvg15"] = round2(m.LoadAvg15)
	}
	if m.UptimeSeconds > 0 {
		r["uptimeHours"] = round2(m.UptimeSeconds / 3600)
	}
	if m.DiskReadIOPS > 0 || m.DiskWriteIOPS > 0 {
		r["diskReadsTotal"] = m.DiskReadIOPS
		r["diskWritesTotal"] = m.DiskWriteIOPS
	}
	return r
}

func round2(v float64) float64 {
	return float64(int(v*100+0.5)) / 100
}

// buildSSHStatus determines healthy/warning/critical based on thresholds.
func buildSSHStatus(m *sshMetrics, check CheckConfig) (status string, messages []string) {
	status = "healthy"
	warn := check.WarningThresholdMs // reuse as generic warning threshold

	if m.CPUUsagePercent > 95 {
		status = "critical"
		messages = append(messages, fmt.Sprintf("CPU critical: %.1f%%", m.CPUUsagePercent))
	} else if m.CPUUsagePercent > 80 {
		if status != "critical" {
			status = "warning"
		}
		messages = append(messages, fmt.Sprintf("CPU high: %.1f%%", m.CPUUsagePercent))
	}

	if m.MemoryUsagePercent > 95 {
		status = "critical"
		messages = append(messages, fmt.Sprintf("Memory critical: %.1f%%", m.MemoryUsagePercent))
	} else if m.MemoryUsagePercent > 85 {
		if status != "critical" {
			status = "warning"
		}
		messages = append(messages, fmt.Sprintf("Memory high: %.1f%%", m.MemoryUsagePercent))
	}

	if m.DiskUsagePercent > 95 {
		status = "critical"
		messages = append(messages, fmt.Sprintf("Disk critical: %.1f%%", m.DiskUsagePercent))
	} else if m.DiskUsagePercent > 85 {
		if status != "critical" {
			status = "warning"
		}
		messages = append(messages, fmt.Sprintf("Disk high: %.1f%%", m.DiskUsagePercent))
	}

	// If custom warning threshold is set, apply to CPU
	if warn > 0 && m.CPUUsagePercent > float64(warn) && status == "healthy" {
		status = "warning"
		messages = append(messages, fmt.Sprintf("CPU above threshold (%d%%): %.1f%%", warn, m.CPUUsagePercent))
	}

	if len(messages) == 0 {
		messages = append(messages, fmt.Sprintf("CPU: %.1f%% | Mem: %.1f%% | Disk: %.1f%% | Load: %.2f",
			m.CPUUsagePercent, m.MemoryUsagePercent, m.DiskUsagePercent, m.LoadAvg1))
	}

	return status, messages
}
