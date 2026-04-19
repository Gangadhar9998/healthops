package monitoring

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Runner struct {
	cfg             *Config
	store           Store
	client          *http.Client
	metrics         *MetricsCollector
	mysqlSampler    MySQLSampler
	mysqlRepo       MySQLMetricsRepository
	mysqlRuleEngine *MySQLRuleEngine
	incidentManager *IncidentManager
	outbox          *FileNotificationOutbox
	snapshotRepo    IncidentSnapshotRepository
	running         bool
	mu              sync.Mutex
}

func NewRunner(cfg *Config, store Store) *Runner {
	return &Runner{
		cfg:     cfg,
		store:   store,
		client:  &http.Client{},
		metrics: nil, // Will be set by SetMetricsCollector
	}
}

// SetMetricsCollector sets the metrics collector for the runner
func (r *Runner) SetMetricsCollector(mc *MetricsCollector) {
	r.metrics = mc
}

func (r *Runner) RunOnce(ctx context.Context) (RunSummary, error) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return RunSummary{Skipped: true}, nil
	}
	r.running = true
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.running = false
		r.mu.Unlock()
	}()

	startedAt := time.Now().UTC()
	state := r.store.Snapshot()
	checks := enabledChecks(state.Checks)
	results := make([]CheckResult, 0, len(checks))

	if len(checks) == 0 {
		finishedAt := time.Now().UTC()
		summary := buildSummary(state.Checks, nil, &finishedAt)
		if err := r.store.SetLastRun(finishedAt); err != nil {
			return RunSummary{}, err
		}
		return RunSummary{StartedAt: startedAt, FinishedAt: finishedAt, Results: nil, Summary: summary}, nil
	}

	jobs := make(chan CheckConfig)
	out := make(chan CheckResult)
	workerCount := r.cfg.Workers
	if workerCount > len(checks) {
		workerCount = len(checks)
	}
	if workerCount <= 0 {
		workerCount = 1
	}

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for check := range jobs {
				result := r.executeCheck(ctx, check)
				out <- result
			}
		}()
	}

	go func() {
		for _, check := range checks {
			jobs <- check
		}
		close(jobs)
		wg.Wait()
		close(out)
	}()

	for result := range out {
		results = append(results, result)
	}

	finishedAt := time.Now().UTC()
	if err := r.persistResults(results, finishedAt); err != nil {
		return RunSummary{}, err
	}

	summary := buildSummary(state.Checks, results, &finishedAt)
	return RunSummary{
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		Results:    results,
		Summary:    summary,
	}, nil
}

// RunCheck executes and persists a single check result.
func (r *Runner) RunCheck(ctx context.Context, check CheckConfig) (CheckResult, error) {
	result := r.executeCheck(ctx, check)
	finishedAt := result.FinishedAt
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
		result.FinishedAt = finishedAt
	}
	if err := r.persistResults([]CheckResult{result}, finishedAt); err != nil {
		return CheckResult{}, err
	}
	return result, nil
}

func (r *Runner) persistResults(results []CheckResult, finishedAt time.Time) error {
	return r.store.Update(func(state *State) error {
		state.Results = append(state.Results, results...)
		state.LastRunAt = finishedAt
		pruneResults(&state.Results, r.cfg.RetentionDays)
		return nil
	})
}

func (r *Runner) executeCheck(ctx context.Context, check CheckConfig) CheckResult {
	startedAt := time.Now().UTC()
	timeout := time.Duration(check.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result := CheckResult{
		ID:          fmt.Sprintf("%s-%d", check.ID, startedAt.UnixNano()),
		CheckID:     check.ID,
		Name:        check.Name,
		Type:        check.Type,
		Server:      check.Server,
		Application: check.Application,
		StartedAt:   startedAt,
		Tags:        cloneTags(check.Tags),
		Metrics:     map[string]float64{},
	}

	var err error
	switch check.Type {
	case "api":
		err = r.runAPI(checkCtx, check, &result)
	case "tcp":
		err = r.runTCP(checkCtx, check, &result)
	case "process":
		err = r.runProcess(checkCtx, check, &result)
	case "command":
		err = r.runCommand(checkCtx, check, &result)
	case "log":
		err = r.runLogFreshness(checkCtx, check, &result)
	case "mysql":
		err = r.runMySQL(checkCtx, check, &result)
	default:
		err = fmt.Errorf("unsupported check type %q", check.Type)
	}

	if err != nil {
		result.Status = "critical"
		result.Healthy = false
		result.Message = err.Error()
	}

	if result.Status == "" {
		result.Status = "healthy"
		result.Healthy = true
	}

	result.DurationMs = time.Since(startedAt).Milliseconds()
	result.FinishedAt = time.Now().UTC()

	// Record metrics if collector is available
	if r.metrics != nil {
		duration := time.Duration(result.DurationMs) * time.Millisecond
		r.metrics.RecordCheckRun(result.CheckID, result.Type, result.Status, duration)
	}

	return result
}

func (r *Runner) runAPI(ctx context.Context, check CheckConfig, result *CheckResult) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, check.Target, nil)
	if err != nil {
		return err
	}

	start := time.Now()
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	result.Metrics["statusCode"] = float64(resp.StatusCode)
	result.Metrics["bodyBytes"] = float64(len(body))
	result.Metrics["latencyMs"] = float64(time.Since(start).Milliseconds())

	if resp.StatusCode != check.ExpectedStatus {
		return fmt.Errorf("unexpected status code %d", resp.StatusCode)
	}
	if check.ExpectedContains != "" && !bytes.Contains(body, []byte(check.ExpectedContains)) {
		return fmt.Errorf("expected response to contain %q", check.ExpectedContains)
	}
	if check.WarningThresholdMs > 0 && result.Metrics["latencyMs"] > float64(check.WarningThresholdMs) {
		result.Status = "warning"
		result.Healthy = false
		result.Message = fmt.Sprintf("slow api response: %.0fms", result.Metrics["latencyMs"])
	}
	return nil
}

func (r *Runner) runTCP(ctx context.Context, check CheckConfig, result *CheckResult) error {
	addr := resolveTCPAddress(check)
	start := time.Now()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	_ = conn.Close()
	result.Metrics["latencyMs"] = float64(time.Since(start).Milliseconds())
	result.Metrics["port"] = float64(check.Port)
	if check.WarningThresholdMs > 0 && result.Metrics["latencyMs"] > float64(check.WarningThresholdMs) {
		result.Status = "warning"
		result.Healthy = false
		result.Message = fmt.Sprintf("slow tcp connect: %.0fms", result.Metrics["latencyMs"])
	}
	return nil
}

func (r *Runner) runProcess(ctx context.Context, check CheckConfig, result *CheckResult) error {
	binary, args := processListCommand()
	cmd := exec.CommandContext(ctx, binary, args...)
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	matched := 0
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, check.Target) {
			matched++
		}
	}

	result.Metrics["matchedProcesses"] = float64(matched)
	if matched == 0 {
		return fmt.Errorf("process %q not found", check.Target)
	}
	return nil
}

// runCommand executes a shell command check.
// SECURITY: This function executes arbitrary shell commands from the config.
// Command checks are only allowed when Config.AllowCommandChecks is explicitly true.
// This function should only be called after validation confirms command checks are enabled.
// The command is executed via 'sh -c' which allows shell operators (pipes, redirects, etc.).
// NEVER pass user input directly into check.Command - config files must be tightly controlled.
func (r *Runner) runCommand(ctx context.Context, check CheckConfig, result *CheckResult) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", check.Command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	result.Metrics["outputBytes"] = float64(len(output))
	if check.ExpectedContains != "" && !bytes.Contains(output, []byte(check.ExpectedContains)) {
		return fmt.Errorf("expected command output to contain %q", check.ExpectedContains)
	}
	return nil
}

func (r *Runner) runLogFreshness(ctx context.Context, check CheckConfig, result *CheckResult) error {
	info, err := os.Stat(check.Path)
	if err != nil {
		return err
	}
	age := time.Since(info.ModTime())
	result.Metrics["ageSeconds"] = age.Seconds()
	if check.FreshnessSeconds > 0 && age > time.Duration(check.FreshnessSeconds)*time.Second {
		return fmt.Errorf("log heartbeat stale: last update %.0fs ago", age.Seconds())
	}
	return nil
}

func enabledChecks(checks []CheckConfig) []CheckConfig {
	out := make([]CheckConfig, 0, len(checks))
	for _, check := range checks {
		if check.IsEnabled() {
			out = append(out, check)
		}
	}
	return out
}

func cloneTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, len(tags))
	copy(out, tags)
	return out
}

func processListCommand() (string, []string) {
	if runtime.GOOS == "darwin" {
		return "ps", []string{"-ax", "-o", "pid=,command="}
	}
	return "ps", []string{"-eo", "pid=,args="}
}

// SetMySQLSampler sets the MySQL sampler for the runner.
func (r *Runner) SetMySQLSampler(sampler MySQLSampler) {
	r.mysqlSampler = sampler
}

// SetMySQLRepo sets the MySQL repository for persisting samples.
func (r *Runner) SetMySQLRepo(repo MySQLMetricsRepository) {
	r.mysqlRepo = repo
}

// SetMySQLRuleEngine sets the MySQL rule engine for alert evaluation.
func (r *Runner) SetMySQLRuleEngine(engine *MySQLRuleEngine) {
	r.mysqlRuleEngine = engine
}

// SetIncidentManager sets the incident manager for rule-triggered incidents.
func (r *Runner) SetIncidentManager(im *IncidentManager) {
	r.incidentManager = im
}

// SetNotificationOutbox sets the notification outbox for alert delivery.
func (r *Runner) SetNotificationOutbox(outbox *FileNotificationOutbox) {
	r.outbox = outbox
}

// SetSnapshotRepo sets the snapshot repository for evidence capture.
func (r *Runner) SetSnapshotRepo(repo IncidentSnapshotRepository) {
	r.snapshotRepo = repo
}

func (r *Runner) runMySQL(ctx context.Context, check CheckConfig, result *CheckResult) error {
	if r.mysqlSampler == nil {
		return fmt.Errorf("mysql sampler not configured")
	}
	if check.MySQL == nil {
		return fmt.Errorf("mysql config block is required")
	}

	sample, err := r.mysqlSampler.Collect(ctx, check)
	if err != nil {
		return err
	}

	result.Metrics["connections"] = float64(sample.Connections)
	result.Metrics["maxConnections"] = float64(sample.MaxConnections)
	result.Metrics["threadsRunning"] = float64(sample.ThreadsRunning)
	result.Metrics["threadsConnected"] = float64(sample.ThreadsConnected)
	result.Metrics["abortedConnects"] = float64(sample.AbortedConnects)
	result.Metrics["slowQueries"] = float64(sample.SlowQueries)
	result.Metrics["questionsPerSec"] = sample.QuestionsPerSec
	result.Metrics["uptimeSeconds"] = float64(sample.UptimeSeconds)
	result.Metrics["innodbRowLockWaits"] = float64(sample.InnoDBRowLockWaits)
	result.Metrics["createdTmpDiskTables"] = float64(sample.CreatedTmpDiskTables)
	result.Metrics["createdTmpTables"] = float64(sample.CreatedTmpTables)
	result.Metrics["threadsCreated"] = float64(sample.ThreadsCreated)
	result.Metrics["maxUsedConnections"] = float64(sample.MaxUsedConnections)

	if sample.MaxConnections > 0 {
		utilPct := float64(sample.Connections) / float64(sample.MaxConnections) * 100
		result.Metrics["connectionUtilPct"] = utilPct
	}

	// Persist sample and compute delta
	var delta *MySQLDelta
	if r.mysqlRepo != nil {
		sampleID, err := r.mysqlRepo.AppendSample(sample)
		if err != nil {
			log.Printf("warning: failed to persist mysql sample: %v", err)
		} else {
			if d, err := r.mysqlRepo.ComputeAndAppendDelta(sampleID); err != nil {
				log.Printf("warning: failed to compute mysql delta: %v", err)
			} else {
				delta = &d
			}
		}
	}

	// Evaluate MySQL rules and process incidents
	if r.mysqlRuleEngine != nil && r.incidentManager != nil {
		evalResults := r.mysqlRuleEngine.Evaluate(check.ID, sample, delta)
		for _, er := range evalResults {
			if er.Action == "open" {
				metadata := map[string]string{
					"ruleCode": er.RuleCode,
					"checkId":  er.CheckID,
				}
				if err := r.incidentManager.ProcessAlert(
					er.CheckID,
					check.Name,
					"mysql",
					er.Severity,
					er.Message,
					metadata,
				); err != nil {
					log.Printf("warning: failed to create incident for rule %s: %v", er.RuleCode, err)
				} else {
					// Record incident ID in rule engine state
					if incident, findErr := r.incidentManager.repo.FindOpenIncident(er.CheckID); findErr == nil && incident.ID != "" {
						r.mysqlRuleEngine.SetOpenIncidentID(er.RuleCode, er.CheckID, incident.ID)

						// Enqueue notification
						if r.outbox != nil {
							notifEvt := NotificationEvent{
								NotificationID: fmt.Sprintf("notif-%s-%d", incident.ID, time.Now().UnixNano()),
								IncidentID:     incident.ID,
								Channel:        "default",
								PayloadJSON:    fmt.Sprintf(`{"severity":"%s","rule":"%s","message":"%s","checkId":"%s"}`, er.Severity, er.RuleCode, er.Message, er.CheckID),
								Status:         "pending",
								CreatedAt:      time.Now().UTC(),
							}
							if err := r.outbox.Enqueue(notifEvt); err != nil {
								log.Printf("warning: failed to enqueue notification: %v", err)
							}
						}

						// Capture evidence snapshots
						if r.snapshotRepo != nil && r.mysqlRepo != nil {
							evidenceCollector := NewMySQLEvidenceCollector(r.mysqlRepo)
							snaps := evidenceCollector.CaptureEvidence(ctx, incident.ID, check.ID, nil)
							if err := r.snapshotRepo.SaveSnapshots(incident.ID, snaps); err != nil {
								log.Printf("warning: failed to save evidence snapshots: %v", err)
							}
						}
					}
				}
			} else if er.Action == "close" && er.IncidentID != "" {
				if err := r.incidentManager.ResolveIncident(er.IncidentID, "system-auto-recovery"); err != nil {
					log.Printf("warning: failed to auto-resolve incident %s: %v", er.IncidentID, err)
				}
			}
		}
	}

	return nil
}
