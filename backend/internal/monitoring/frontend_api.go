package monitoring

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Response types
// ---------------------------------------------------------------------------

// SafeConfigView exposes runtime config without auth credentials.
type SafeConfigView struct {
	Server               ServerConfig `json:"server"`
	AuthEnabled          bool         `json:"authEnabled"`
	RetentionDays        int          `json:"retentionDays"`
	CheckIntervalSeconds int          `json:"checkIntervalSeconds"`
	Workers              int          `json:"workers"`
	AllowCommandChecks   bool         `json:"allowCommandChecks"`
	TotalChecks          int          `json:"totalChecks"`
	TotalServers         int          `json:"totalServers"`
}

// ConfigUpdate carries the subset of config fields safe to change at runtime.
type ConfigUpdate struct {
	RetentionDays        *int  `json:"retentionDays,omitempty"`
	CheckIntervalSeconds *int  `json:"checkIntervalSeconds,omitempty"`
	Workers              *int  `json:"workers,omitempty"`
	AllowCommandChecks   *bool `json:"allowCommandChecks,omitempty"`
}

// IncidentFilter holds query-string parameters for incident listing.
type IncidentFilter struct {
	Status   string
	Severity string
	CheckID  string
	Limit    int
	Offset   int
}

// SSEPayload is the JSON body sent in each SSE event.
type SSEPayload struct {
	Type            string    `json:"type"`
	Timestamp       time.Time `json:"timestamp"`
	Summary         Summary   `json:"summary"`
	ActiveIncidents int       `json:"activeIncidents"`
}

// AuthInfo returns the caller's identity.
type AuthInfo struct {
	Username    string `json:"username"`
	AuthEnabled bool   `json:"authEnabled"`
}

// ---------------------------------------------------------------------------
// AlertRuleEngine helpers (same package — white-box access)
// ---------------------------------------------------------------------------

// Rules returns a copy of all configured alert rules.
func (e *AlertRuleEngine) Rules() []AlertRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]AlertRule, len(e.rules))
	copy(out, e.rules)
	return out
}

// AddRule appends a new rule to the engine.
func (e *AlertRuleEngine) AddRule(rule AlertRule) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, rule)
	e.persist()
}

// GetRule returns a single alert rule by ID.
func (e *AlertRuleEngine) GetRule(id string) (AlertRule, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, r := range e.rules {
		if r.ID == id {
			return r, nil
		}
	}
	return AlertRule{}, fmt.Errorf("alert rule not found: %s", id)
}

// UpdateRule replaces the rule with a matching ID.
func (e *AlertRuleEngine) UpdateRule(rule AlertRule) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, r := range e.rules {
		if r.ID == rule.ID {
			e.rules[i] = rule
			e.persist()
			return nil
		}
	}
	return fmt.Errorf("alert rule not found: %s", rule.ID)
}

// DeleteRule removes the rule with the given ID.
func (e *AlertRuleEngine) DeleteRule(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i, r := range e.rules {
		if r.ID == id {
			e.rules = append(e.rules[:i], e.rules[i+1:]...)
			e.persist()
			return nil
		}
	}
	return fmt.Errorf("alert rule not found: %s", id)
}

// ---------------------------------------------------------------------------
// 1. Config endpoints
// ---------------------------------------------------------------------------

func (s *Service) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleConfigGet(w, r)
	case http.MethodPut:
		s.handleConfigPut(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Service) handleConfigGet(w http.ResponseWriter, _ *http.Request) {
	view := SafeConfigView{
		Server:               s.cfg.Server,
		AuthEnabled:          s.cfg.Auth.Enabled,
		RetentionDays:        s.cfg.RetentionDays,
		CheckIntervalSeconds: s.cfg.CheckIntervalSeconds,
		Workers:              s.cfg.Workers,
		AllowCommandChecks:   s.cfg.AllowCommandChecks,
		TotalChecks:          len(s.cfg.Checks),
		TotalServers:         len(s.cfg.Servers),
	}
	WriteAPIResponse(w, http.StatusOK, NewAPIResponse(view))
}

func (s *Service) handleConfigPut(w http.ResponseWriter, r *http.Request) {
	if !IsRequestAuthorized(s.cfg.Auth, r) {
		RequestAuth(w)
		return
	}

	var update ConfigUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}

	// Validate
	if update.RetentionDays != nil {
		if *update.RetentionDays < 1 || *update.RetentionDays > 365 {
			WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("retentionDays must be 1-365"))
			return
		}
	}
	if update.CheckIntervalSeconds != nil {
		if *update.CheckIntervalSeconds < 5 || *update.CheckIntervalSeconds > 3600 {
			WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("checkIntervalSeconds must be 5-3600"))
			return
		}
	}
	if update.Workers != nil {
		if *update.Workers < 1 || *update.Workers > 100 {
			WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("workers must be 1-100"))
			return
		}
	}

	// Apply
	details := map[string]interface{}{}
	if update.RetentionDays != nil {
		details["retentionDays"] = map[string]int{"old": s.cfg.RetentionDays, "new": *update.RetentionDays}
		s.cfg.RetentionDays = *update.RetentionDays
	}
	if update.CheckIntervalSeconds != nil {
		details["checkIntervalSeconds"] = map[string]int{"old": s.cfg.CheckIntervalSeconds, "new": *update.CheckIntervalSeconds}
		s.cfg.CheckIntervalSeconds = *update.CheckIntervalSeconds
	}
	if update.Workers != nil {
		details["workers"] = map[string]int{"old": s.cfg.Workers, "new": *update.Workers}
		s.cfg.Workers = *update.Workers
	}
	if update.AllowCommandChecks != nil {
		details["allowCommandChecks"] = map[string]bool{"old": s.cfg.AllowCommandChecks, "new": *update.AllowCommandChecks}
		s.cfg.AllowCommandChecks = *update.AllowCommandChecks
	}

	if s.auditLogger != nil {
		actor := ExtractActorFromRequest(r, s.cfg)
		_ = s.auditLogger.Log("config.updated", actor, "config", "", details)
	}

	WriteAPIResponse(w, http.StatusOK, NewAPIResponse(s.safeConfigView()))
}

func (s *Service) safeConfigView() SafeConfigView {
	return SafeConfigView{
		Server:               s.cfg.Server,
		AuthEnabled:          s.cfg.Auth.Enabled,
		RetentionDays:        s.cfg.RetentionDays,
		CheckIntervalSeconds: s.cfg.CheckIntervalSeconds,
		Workers:              s.cfg.Workers,
		AllowCommandChecks:   s.cfg.AllowCommandChecks,
		TotalChecks:          len(s.cfg.Checks),
		TotalServers:         len(s.cfg.Servers),
	}
}

// ---------------------------------------------------------------------------
// 2. Alert Rules endpoints
// ---------------------------------------------------------------------------

func (s *Service) handleAlertRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rules := s.alertEngine.Rules()
		WriteAPIResponse(w, http.StatusOK, NewAPIResponse(rules))
	case http.MethodPost:
		s.handleAlertRuleCreate(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Service) handleAlertRuleCreate(w http.ResponseWriter, r *http.Request) {
	if !IsRequestAuthorized(s.cfg.Auth, r) {
		RequestAuth(w)
		return
	}

	var rule AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}

	if rule.ID == "" || rule.Name == "" || rule.Severity == "" {
		WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("id, name, and severity are required"))
		return
	}

	s.alertEngine.AddRule(rule)

	if s.auditLogger != nil {
		actor := ExtractActorFromRequest(r, s.cfg)
		_ = s.auditLogger.Log("alertrule.created", actor, "alertRule", rule.ID, map[string]interface{}{
			"name":     rule.Name,
			"severity": rule.Severity,
		})
	}

	WriteAPIResponse(w, http.StatusCreated, NewAPIResponse(rule))
}

func (s *Service) handleAlertRuleByID(w http.ResponseWriter, r *http.Request) {
	// Extract ID from path: /api/v1/alert-rules/{id}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/alert-rules/"), "/")
	id := parts[0]
	if id == "" {
		WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("rule id is required"))
		return
	}

	switch r.Method {
	case http.MethodGet:
		s.handleAlertRuleGet(w, r, id)
	case http.MethodPut:
		s.handleAlertRuleUpdate(w, r, id)
	case http.MethodDelete:
		s.handleAlertRuleDelete(w, r, id)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Service) handleAlertRuleGet(w http.ResponseWriter, _ *http.Request, id string) {
	if s.alertEngine == nil {
		WriteAPIError(w, http.StatusServiceUnavailable, fmt.Errorf("alert engine not configured"))
		return
	}
	rule, err := s.alertEngine.GetRule(id)
	if err != nil {
		WriteAPIError(w, http.StatusNotFound, err)
		return
	}
	WriteAPIResponse(w, http.StatusOK, NewAPIResponse(rule))
}

func (s *Service) handleAlertRuleUpdate(w http.ResponseWriter, r *http.Request, id string) {
	if !IsRequestAuthorized(s.cfg.Auth, r) {
		RequestAuth(w)
		return
	}

	var rule AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("invalid JSON: %w", err))
		return
	}

	rule.ID = id // ensure path ID takes precedence
	if err := s.alertEngine.UpdateRule(rule); err != nil {
		WriteAPIError(w, http.StatusNotFound, err)
		return
	}

	if s.auditLogger != nil {
		actor := ExtractActorFromRequest(r, s.cfg)
		_ = s.auditLogger.Log("alertrule.updated", actor, "alertRule", id, map[string]interface{}{
			"name":     rule.Name,
			"severity": rule.Severity,
		})
	}

	WriteAPIResponse(w, http.StatusOK, NewAPIResponse(rule))
}

func (s *Service) handleAlertRuleDelete(w http.ResponseWriter, r *http.Request, id string) {
	if !IsRequestAuthorized(s.cfg.Auth, r) {
		RequestAuth(w)
		return
	}

	if err := s.alertEngine.DeleteRule(id); err != nil {
		WriteAPIError(w, http.StatusNotFound, err)
		return
	}

	if s.auditLogger != nil {
		actor := ExtractActorFromRequest(r, s.cfg)
		_ = s.auditLogger.Log("alertrule.deleted", actor, "alertRule", id, nil)
	}

	WriteAPIResponse(w, http.StatusOK, NewAPIResponse(map[string]string{"deleted": id}))
}

// ---------------------------------------------------------------------------
// 3. Incident filtering
// ---------------------------------------------------------------------------

func (s *Service) handleIncidentsFiltered(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filter := IncidentFilter{
		Status:   r.URL.Query().Get("status"),
		Severity: r.URL.Query().Get("severity"),
		CheckID:  r.URL.Query().Get("checkId"),
		Limit:    QueryInt(r, "limit", 50),
		Offset:   QueryInt(r, "offset", 0),
	}

	if filter.Limit < 1 {
		filter.Limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	if s.incidentManager == nil {
		WriteAPIResponse(w, http.StatusOK, NewPaginatedResponse([]Incident{}, 0, filter.Limit, filter.Offset))
		return
	}

	incidents, err := s.incidentManager.repo.ListIncidents()
	if err != nil {
		WriteAPIError(w, http.StatusInternalServerError, fmt.Errorf("list incidents: %w", err))
		return
	}

	// Apply filters
	filtered := make([]Incident, 0, len(incidents))
	for _, inc := range incidents {
		if filter.Status != "" && !strings.EqualFold(inc.Status, filter.Status) {
			continue
		}
		if filter.Severity != "" && !strings.EqualFold(inc.Severity, filter.Severity) {
			continue
		}
		if filter.CheckID != "" && inc.CheckID != filter.CheckID {
			continue
		}
		filtered = append(filtered, inc)
	}

	total := len(filtered)

	// Pagination
	start := filter.Offset
	if start > total {
		start = total
	}
	end := start + filter.Limit
	if end > total {
		end = total
	}
	page := filtered[start:end]

	WriteAPIResponse(w, http.StatusOK, NewPaginatedResponse(page, total, filter.Limit, filter.Offset))
}

// ---------------------------------------------------------------------------
// 4. SSE (Server-Sent Events)
// ---------------------------------------------------------------------------

func (s *Service) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteAPIError(w, http.StatusInternalServerError, fmt.Errorf("streaming not supported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Send initial snapshot immediately
	s.sendSSEEvent(w, flusher, "snapshot")

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			s.sendSSEEvent(w, flusher, "snapshot")
		}
	}
}

func (s *Service) sendSSEEvent(w http.ResponseWriter, flusher http.Flusher, eventType string) {
	snap := s.store.DashboardSnapshot()

	activeIncidents := 0
	if s.incidentManager != nil {
		if incidents, err := s.incidentManager.repo.ListIncidents(); err == nil {
			for _, inc := range incidents {
				if inc.Status == "open" || inc.Status == "acknowledged" {
					activeIncidents++
				}
			}
		}
	}

	payload := SSEPayload{
		Type:            eventType,
		Timestamp:       time.Now().UTC(),
		Summary:         snap.Summary,
		ActiveIncidents: activeIncidents,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, data)
	flusher.Flush()
}

// ---------------------------------------------------------------------------
// 5. Auth info
// ---------------------------------------------------------------------------

func (s *Service) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Try JWT first
	if s.userAPI != nil {
		s.userAPI.HandleMe(w, r)
		return
	}

	// Legacy basic auth
	info := AuthInfo{
		Username:    ExtractActorFromRequest(r, s.cfg),
		AuthEnabled: s.cfg.Auth.Enabled,
	}
	WriteAPIResponse(w, http.StatusOK, NewAPIResponse(info))
}
