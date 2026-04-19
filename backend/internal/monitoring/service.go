package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Service struct {
	cfg             *Config
	store           Store
	runner          *Runner
	scheduler       *CheckScheduler
	incidentManager *IncidentManager
	alertEngine     *AlertRuleEngine
	metrics         *MetricsCollector
	logger          *log.Logger
	auditLogger     *AuditLogger
	mysqlAPIHandler *MySQLAPIHandler
	aiAPIHandler    *AIAPIHandler
}

func NewService(cfg *Config, store Store, logger *log.Logger) *Service {
	hasLogger := logger != nil
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}

	runner := NewRunner(cfg, store)
	metrics := NewMetricsCollector()
	runner.SetMetricsCollector(metrics)

	svc := &Service{
		cfg:       cfg,
		store:     store,
		runner:    runner,
		scheduler: NewCheckScheduler(cfg, store, runner, logger),
		metrics:   metrics,
		logger:    logger,
	}

	// Initialize audit logger
	if hasLogger {
		auditRepo, err := NewFileAuditRepository("data/audit.json")
		if err != nil {
			logger.Printf("Warning: Failed to initialize audit logger: %v", err)
		} else {
			svc.auditLogger = NewAuditLogger(auditRepo, logger)
		}
	}

	// Initialize alert rule engine with empty rules (can be loaded from config later)
	alertRules, _ := LoadRulesFromConfig(cfg)
	if logger != nil {
		svc.alertEngine = NewAlertRuleEngine(alertRules, logger)
	}

	// Set up alert callback for scheduler
	svc.scheduler.SetAlertCallback(func(results []CheckResult) {
		if svc.alertEngine != nil {
			alerts := svc.alertEngine.Evaluate(results)
			if len(alerts) > 0 && svc.logger != nil {
				svc.logger.Printf("alert evaluation: %d alerts triggered", len(alerts))

				// Process alerts through incident manager if configured
				if svc.incidentManager != nil {
					for _, alert := range alerts {
						metadata := map[string]string{
							"ruleId":   alert.RuleID,
							"ruleName": alert.RuleName,
							"message":  alert.Message,
						}
						_ = svc.incidentManager.ProcessAlert(
							alert.CheckID,
							alert.CheckName,
							"", // type not available in alert
							alert.Severity,
							alert.Message,
							metadata,
						)
					}
				}
			}
		}
	})

	return svc
}

// SetIncidentManager sets the incident manager for the service
func (s *Service) SetIncidentManager(im *IncidentManager) {
	s.incidentManager = im
}

// SetAuditLogger sets the audit logger for the service
func (s *Service) SetAuditLogger(al *AuditLogger) {
	s.auditLogger = al
}

// SetAlertEngine sets the alert rule engine for the service
func (s *Service) SetAlertEngine(ae *AlertRuleEngine) {
	s.alertEngine = ae
}

// SetMySQLAPIHandler sets the MySQL API handler for the service
func (s *Service) SetMySQLAPIHandler(h *MySQLAPIHandler) {
	s.mysqlAPIHandler = h
}

// SetAIAPIHandler sets the AI API handler for the service
func (s *Service) SetAIAPIHandler(h *AIAPIHandler) {
	s.aiAPIHandler = h
}

// Runner returns the service's runner for external configuration.
func (s *Service) Runner() *Runner {
	return s.runner
}

func (s *Service) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/readyz", s.handleReadyz)
	mux.HandleFunc("/api/v1/checks", s.handleChecks)
	mux.HandleFunc("/api/v1/checks/", s.handleCheckByID)
	mux.HandleFunc("/api/v1/runs", s.handleRun)
	mux.HandleFunc("/api/v1/summary", s.handleSummary)
	mux.HandleFunc("/api/v1/results", s.handleResults)
	mux.HandleFunc("/api/v1/dashboard", s.handleDashboard)
	mux.HandleFunc("/api/v1/dashboard/checks", s.handleDashboardChecks)
	mux.HandleFunc("/api/v1/dashboard/summary", s.handleDashboardSummary)
	mux.HandleFunc("/api/v1/dashboard/results", s.handleDashboardResults)
	mux.HandleFunc("/api/v1/incidents", s.handleIncidentsFiltered)
	mux.HandleFunc("/api/v1/incidents/", s.handleIncidentByID)
	mux.HandleFunc("/api/v1/audit", s.handleAudit)

	// Analytics and stats endpoints
	mux.HandleFunc("/api/v1/analytics/uptime", s.handleAnalyticsUptime)
	mux.HandleFunc("/api/v1/analytics/response-times", s.handleAnalyticsResponseTimes)
	mux.HandleFunc("/api/v1/analytics/status-timeline", s.handleAnalyticsStatusTimeline)
	mux.HandleFunc("/api/v1/analytics/failure-rate", s.handleAnalyticsFailureRate)
	mux.HandleFunc("/api/v1/analytics/incidents", s.handleAnalyticsMTTR)
	mux.HandleFunc("/api/v1/stats/overview", s.handleStatsOverview)

	// Config and alert rules
	mux.HandleFunc("/api/v1/config", s.handleConfig)
	mux.HandleFunc("/api/v1/alert-rules", s.handleAlertRules)
	mux.HandleFunc("/api/v1/alert-rules/", s.handleAlertRuleByID)

	// SSE and auth
	mux.HandleFunc("/api/v1/events", s.handleSSE)
	mux.HandleFunc("/api/v1/auth/me", s.handleAuthMe)

	// Export endpoints
	if s.incidentManager != nil {
		mux.HandleFunc("/api/v1/export/incidents", handleExportIncidents(s.incidentManager.repo))
	}
	mux.HandleFunc("/api/v1/export/results", handleExportResults(s.store, s.cfg.RetentionDays))

	// Register MySQL/generic API routes if handler is configured
	if s.mysqlAPIHandler != nil {
		s.mysqlAPIHandler.RegisterRoutes(mux)
		s.mysqlAPIHandler.RegisterAnalyticsRoutes(mux)
	}

	// Register AI API routes if handler is configured
	if s.aiAPIHandler != nil {
		s.aiAPIHandler.RegisterRoutes(mux)
	}

	// Add Prometheus metrics endpoint
	mux.Handle("/metrics", s.metrics.Handler())

	// Serve frontend SPA if the dist directory exists
	frontendDir := os.Getenv("FRONTEND_DIR")
	if frontendDir == "" {
		frontendDir = "frontend/dist"
	}
	if spaHandler := NewSPAHandler(frontendDir); spaHandler != nil {
		mux.Handle("/", spaHandler)
		s.logger.Printf("serving frontend from %s", frontendDir)
	}

	// Apply middlewares: first metrics, then logging
	var handler http.Handler = mux
	handler = metricsMiddleware(s.metrics, handler)
	handler = loggingMiddleware(s.logger, handler)
	handler = basicAuthMiddleware(s.cfg.Auth, handler)

	server := &http.Server{
		Addr:              s.cfg.Server.Addr,
		Handler:           handler,
		ReadTimeout:       time.Duration(s.cfg.Server.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:      0, // Disabled: AI endpoints need long write windows; per-handler timeouts via context
		IdleTimeout:       time.Duration(s.cfg.Server.IdleTimeoutSeconds) * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Printf("HTTP listening on %s", s.cfg.Server.Addr)
		errCh <- server.ListenAndServe()
	}()

	s.scheduler.Start()
	defer s.scheduler.Stop()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Service) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeAPIResponse(w, http.StatusOK, NewAPIResponse(map[string]string{"status": "ok"}))
}

func (s *Service) handleReadyz(w http.ResponseWriter, r *http.Request) {
	state := s.store.Snapshot()
	writeAPIResponse(w, http.StatusOK, NewAPIResponse(map[string]interface{}{
		"status":    "ready",
		"checks":    len(state.Checks),
		"lastRunAt": state.LastRunAt,
	}))
}

func (s *Service) handleChecks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		state := s.store.Snapshot()
		items := toCheckListItems(state.Checks)
		writeAPIResponse(w, http.StatusOK, NewAPIResponse(items))
	case http.MethodPost:
		if !isRequestAuthorized(s.cfg.Auth, r) {
			requestAuth(w)
			return
		}
		var check CheckConfig
		if err := json.NewDecoder(r.Body).Decode(&check); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		check.applyDefaults()
		if check.ID == "" {
			check.ID = buildCheckID(&check)
		}
		if err := check.validate(s.cfg); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		if err := s.store.UpsertCheck(check); err != nil {
			writeAPIError(w, http.StatusInternalServerError, err)
			return
		}
		s.scheduler.UpsertSchedule(check)

		// Audit log
		if s.auditLogger != nil {
			actor := ExtractActorFromRequest(r, s.cfg)
			_ = s.auditLogger.Log("check.created", actor, "check", check.ID, map[string]interface{}{
				"name": check.Name,
				"type": check.Type,
			})
		}

		writeAPIResponse(w, http.StatusCreated, NewAPIResponse(check))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Service) handleCheckByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/checks/")
	if id == "" {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("missing check id"))
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleGetCheck(w, r, id)
		return
	case http.MethodPut, http.MethodPatch:
		if !isRequestAuthorized(s.cfg.Auth, r) {
			requestAuth(w)
			return
		}
		var check CheckConfig
		if err := json.NewDecoder(r.Body).Decode(&check); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		check.ID = id
		check.applyDefaults()
		if err := check.validate(s.cfg); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		if err := s.store.UpsertCheck(check); err != nil {
			writeAPIError(w, http.StatusInternalServerError, err)
			return
		}
		s.scheduler.UpsertSchedule(check)

		// Audit log
		if s.auditLogger != nil {
			actor := ExtractActorFromRequest(r, s.cfg)
			_ = s.auditLogger.Log("check.updated", actor, "check", id, map[string]interface{}{
				"name": check.Name,
				"type": check.Type,
			})
		}

		writeAPIResponse(w, http.StatusOK, NewAPIResponse(check))
	case http.MethodDelete:
		if !isRequestAuthorized(s.cfg.Auth, r) {
			requestAuth(w)
			return
		}
		if err := s.store.DeleteCheck(id); err != nil {
			writeAPIError(w, http.StatusInternalServerError, err)
			return
		}
		s.scheduler.RemoveSchedule(id)

		// Audit log
		if s.auditLogger != nil {
			actor := ExtractActorFromRequest(r, s.cfg)
			_ = s.auditLogger.Log("check.deleted", actor, "check", id, nil)
		}

		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Service) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !isRequestAuthorized(s.cfg.Auth, r) {
		requestAuth(w)
		return
	}

	// Check if this is a single check run or full run
	var runSpec struct {
		CheckID string `json:"checkId,omitempty"`
	}

	// Try to decode request body (may be empty for full run)
	decoder := json.NewDecoder(r.Body)
	_ = decoder.Decode(&runSpec)

	// Single check run
	if runSpec.CheckID != "" {
		// Find the check in the snapshot
		state := s.store.Snapshot()
		var check *CheckConfig
		for i := range state.Checks {
			if state.Checks[i].ID == runSpec.CheckID {
				check = &state.Checks[i]
				break
			}
		}

		if check == nil {
			writeAPIError(w, http.StatusNotFound, fmt.Errorf("check not found: %s", runSpec.CheckID))
			return
		}

		// Execute and persist single check.
		result, err := s.runner.RunCheck(r.Context(), *check)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, err)
			return
		}

		// Evaluate alert rules
		if s.alertEngine != nil {
			alerts := s.alertEngine.Evaluate([]CheckResult{result})
			s.logger.Printf("single check run: %d alerts triggered", len(alerts))

			if s.incidentManager != nil && len(alerts) > 0 {
				for _, alert := range alerts {
					metadata := map[string]string{
						"ruleId":   alert.RuleID,
						"ruleName": alert.RuleName,
						"message":  alert.Message,
					}
					_ = s.incidentManager.ProcessAlert(
						alert.CheckID,
						alert.CheckName,
						check.Type,
						alert.Severity,
						alert.Message,
						metadata,
					)
				}
			}
		}

		writeAPIResponse(w, http.StatusAccepted, NewAPIResponse(result))
		return
	}

	// Full run of all checks
	summary, err := s.runner.RunOnce(r.Context())
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}

	// Evaluate alert rules against check results
	if s.alertEngine != nil && len(summary.Results) > 0 {
		alerts := s.alertEngine.Evaluate(summary.Results)
		s.logger.Printf("alert evaluation: %d alerts triggered", len(alerts))

		// Process alerts through incident manager if configured
		if s.incidentManager != nil {
			for _, alert := range alerts {
				metadata := map[string]string{
					"ruleId":   alert.RuleID,
					"ruleName": alert.RuleName,
					"message":  alert.Message,
				}
				_ = s.incidentManager.ProcessAlert(
					alert.CheckID,
					alert.CheckName,
					"", // type not available in alert
					alert.Severity,
					alert.Message,
					metadata,
				)
			}
		}
	}

	writeAPIResponse(w, http.StatusAccepted, NewAPIResponse(summary))
}

func (s *Service) handleSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	state := s.store.Snapshot()
	summary := buildSummary(state.Checks, state.Results, &state.LastRunAt)
	writeAPIResponse(w, http.StatusOK, NewAPIResponse(summary))
}

func (s *Service) handleResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	state := s.store.Snapshot()
	checkID := strings.TrimSpace(r.URL.Query().Get("checkId"))
	days := queryInt(r, "days", s.cfg.RetentionDays)
	results := filterResults(state.Results, checkID, days)
	writeAPIResponse(w, http.StatusOK, NewAPIResponse(results))
}

func (s *Service) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	snapshot := s.store.DashboardSnapshot()
	writeAPIResponse(w, http.StatusOK, NewAPIResponse(snapshot))
}

func (s *Service) handleDashboardChecks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	snapshot := s.store.DashboardSnapshot()
	items := toCheckListItems(snapshot.State.Checks)
	writeAPIResponse(w, http.StatusOK, NewAPIResponse(items))
}

func (s *Service) handleDashboardSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	snapshot := s.store.DashboardSnapshot()
	writeAPIResponse(w, http.StatusOK, NewAPIResponse(snapshot.Summary))
}

func (s *Service) handleDashboardResults(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	snapshot := s.store.DashboardSnapshot()
	checkID := strings.TrimSpace(r.URL.Query().Get("checkId"))
	days := queryInt(r, "days", s.cfg.RetentionDays)
	results := filterResults(snapshot.State.Results, checkID, days)
	writeAPIResponse(w, http.StatusOK, NewAPIResponse(results))
}

func filterResults(results []CheckResult, checkID string, days int) []CheckResult {
	if days <= 0 {
		days = 7
	}
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	out := make([]CheckResult, 0, len(results))
	for _, result := range results {
		finishedAt := result.FinishedAt
		if finishedAt.IsZero() {
			finishedAt = result.StartedAt
		}
		if !finishedAt.IsZero() && finishedAt.Before(cutoff) {
			continue
		}
		if checkID != "" && result.CheckID != checkID {
			continue
		}
		out = append(out, result)
	}
	return out
}

func buildSummary(checks []CheckConfig, results []CheckResult, lastRunAt *time.Time) Summary {
	latestByID := make(map[string]CheckResult, len(results))
	for _, result := range results {
		latestByID[result.CheckID] = result
	}

	latest := make([]CheckResult, 0, len(checks))
	byServer := map[string]StatusCount{}
	byApp := map[string]StatusCount{}
	summary := Summary{
		TotalChecks:   len(checks),
		ByServer:      byServer,
		ByApplication: byApp,
	}
	if lastRunAt != nil && !lastRunAt.IsZero() {
		copy := lastRunAt.UTC()
		summary.LastRunAt = &copy
	}

	for _, check := range checks {
		result, ok := latestByID[check.ID]
		if !ok {
			result = CheckResult{
				CheckID:     check.ID,
				Name:        check.Name,
				Type:        check.Type,
				Server:      check.Server,
				Application: check.Application,
				Status:      "unknown",
				Healthy:     false,
				Message:     "check has not run yet",
				Tags:        cloneTags(check.Tags),
			}
		}
		latest = append(latest, result)
		if check.IsEnabled() {
			summary.EnabledChecks++
		}
		accumulateCount(&summary, result)
		addGroupCount(byServer, check.Server, result.Status)
		addGroupCount(byApp, check.Application, result.Status)
	}

	summary.Latest = latest
	return summary
}

func accumulateCount(summary *Summary, result CheckResult) {
	switch result.Status {
	case "healthy":
		summary.Healthy++
	case "warning":
		summary.Warning++
	case "critical":
		summary.Critical++
	default:
		summary.Unknown++
	}
}

func addGroupCount(groups map[string]StatusCount, key, status string) {
	if key == "" {
		key = "default"
	}
	current := groups[key]
	current.Total++
	switch status {
	case "healthy":
		current.Healthy++
	case "warning":
		current.Warning++
	case "critical":
		current.Critical++
	default:
		current.Unknown++
	}
	groups[key] = current
}

func queryInt(r *http.Request, key string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func loggingMiddleware(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		logger.Printf("%s %s %s", r.Method, r.URL.Path, duration.Round(time.Millisecond))

		// Note: Metrics recording is now done by metricsMiddleware
		// This middleware only handles logging
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(statusCode int) {
	w.status = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

// Flush implements http.Flusher so SSE streaming works through middleware.
func (w *responseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// metricsMiddleware records HTTP request metrics
func metricsMiddleware(mc *MetricsCollector, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code
		wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)

		// Record metrics if collector is available
		if mc != nil {
			mc.RecordHTTPRequest(r.Method, r.URL.Path, wrapped.status, duration)
		}
	})
}

func (s *Service) handleIncidents(w http.ResponseWriter, r *http.Request) {
	if s.incidentManager == nil {
		writeAPIError(w, http.StatusServiceUnavailable, fmt.Errorf("incident manager not configured"))
		return
	}

	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Get the incident repo from the incident manager
	repo := s.incidentManager.repo
	incidents, err := repo.ListIncidents()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}

	writeAPIResponse(w, http.StatusOK, NewAPIResponse(incidents))
}

func (s *Service) handleIncidentByID(w http.ResponseWriter, r *http.Request) {
	if s.incidentManager == nil {
		writeAPIError(w, http.StatusServiceUnavailable, fmt.Errorf("incident manager not configured"))
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/incidents/")
	parts := strings.Split(path, "/")
	incidentID := parts[0]

	if incidentID == "" {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("missing incident id"))
		return
	}

	switch r.Method {
	case http.MethodGet:
		if len(parts) >= 2 && parts[1] == "snapshots" {
			s.getIncidentSnapshots(w, incidentID)
			return
		}
		s.getIncident(w, incidentID)
	case http.MethodPost:
		if !isRequestAuthorized(s.cfg.Auth, r) {
			requestAuth(w)
			return
		}
		if len(parts) >= 2 {
			action := parts[1]
			switch action {
			case "acknowledge":
				s.acknowledgeIncident(w, r, incidentID)
			case "resolve":
				s.resolveIncident(w, r, incidentID)
			default:
				writeAPIError(w, http.StatusBadRequest, fmt.Errorf("unknown action: %s", action))
			}
		} else {
			writeAPIError(w, http.StatusBadRequest, fmt.Errorf("missing action"))
		}
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Service) getIncident(w http.ResponseWriter, incidentID string) {
	repo := s.incidentManager.repo
	incident, err := repo.GetIncident(incidentID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}

	if incident.ID == "" {
		writeAPIError(w, http.StatusNotFound, fmt.Errorf("incident not found"))
		return
	}

	writeAPIResponse(w, http.StatusOK, NewAPIResponse(incident))
}

func (s *Service) getIncidentSnapshots(w http.ResponseWriter, incidentID string) {
	if s.mysqlAPIHandler != nil && s.mysqlAPIHandler.snapshotRepo != nil {
		snaps, err := s.mysqlAPIHandler.snapshotRepo.GetSnapshots(incidentID)
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, err)
			return
		}
		writeAPIResponse(w, http.StatusOK, NewAPIResponse(snaps))
		return
	}
	writeAPIResponse(w, http.StatusOK, NewAPIResponse([]struct{}{}))
}

func (s *Service) acknowledgeIncident(w http.ResponseWriter, r *http.Request, incidentID string) {
	var payload struct {
		AcknowledgedBy string `json:"acknowledgedBy"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}

	if payload.AcknowledgedBy == "" {
		payload.AcknowledgedBy = "anonymous"
	}

	if err := s.incidentManager.AcknowledgeIncident(incidentID, payload.AcknowledgedBy); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}

	// Audit log
	if s.auditLogger != nil {
		actor := ExtractActorFromRequest(r, s.cfg)
		_ = s.auditLogger.Log("incident.acknowledged", actor, "incident", incidentID, map[string]interface{}{
			"acknowledgedBy": payload.AcknowledgedBy,
		})
	}

	// Return the updated incident
	s.getIncident(w, incidentID)
}

func (s *Service) resolveIncident(w http.ResponseWriter, r *http.Request, incidentID string) {
	var payload struct {
		ResolvedBy string `json:"resolvedBy"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}

	if payload.ResolvedBy == "" {
		payload.ResolvedBy = "anonymous"
	}

	if err := s.incidentManager.ResolveIncident(incidentID, payload.ResolvedBy); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}

	// Audit log
	if s.auditLogger != nil {
		actor := ExtractActorFromRequest(r, s.cfg)
		_ = s.auditLogger.Log("incident.resolved", actor, "incident", incidentID, map[string]interface{}{
			"resolvedBy": payload.ResolvedBy,
		})
	}

	// Return the updated incident
	s.getIncident(w, incidentID)
}

func (s *Service) handleAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if s.auditLogger == nil {
		writeAPIError(w, http.StatusServiceUnavailable, fmt.Errorf("audit logger not configured"))
		return
	}

	// Parse query parameters
	filter := AuditFilter{
		Action:   strings.TrimSpace(r.URL.Query().Get("action")),
		Actor:    strings.TrimSpace(r.URL.Query().Get("actor")),
		Target:   strings.TrimSpace(r.URL.Query().Get("target")),
		TargetID: strings.TrimSpace(r.URL.Query().Get("targetId")),
		Limit:    queryInt(r, "limit", 100),
		Offset:   queryInt(r, "offset", 0),
	}

	// Parse time range if provided
	if startTime := strings.TrimSpace(r.URL.Query().Get("startTime")); startTime != "" {
		if t, err := time.Parse(time.RFC3339, startTime); err == nil {
			filter.StartTime = t
		}
	}
	if endTime := strings.TrimSpace(r.URL.Query().Get("endTime")); endTime != "" {
		if t, err := time.Parse(time.RFC3339, endTime); err == nil {
			filter.EndTime = t
		}
	}

	events, err := s.auditLogger.GetAuditEvents(filter)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}

	writeAPIResponse(w, http.StatusOK, NewAPIResponse(events))
}
