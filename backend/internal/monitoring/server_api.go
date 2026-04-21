package monitoring

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// sanitizeServerForAPI masks sensitive fields before returning to API clients.
func sanitizeServerForAPI(s RemoteServer) RemoteServer {
	if s.Password != "" {
		s.Password = "********"
	}
	return s
}

func sanitizeServersForAPI(servers []RemoteServer) []RemoteServer {
	out := make([]RemoteServer, len(servers))
	for i, s := range servers {
		out[i] = sanitizeServerForAPI(s)
	}
	return out
}

func (s *Service) handleServers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		WriteAPIResponse(w, http.StatusOK, NewAPIResponse(sanitizeServersForAPI(s.cfg.Servers)))

	case http.MethodPost:
		if !IsRequestAuthorized(s.cfg.Auth, r) {
			RequestAuth(w)
			return
		}
		var srv RemoteServer
		if err := json.NewDecoder(r.Body).Decode(&srv); err != nil {
			WriteAPIError(w, http.StatusBadRequest, err)
			return
		}
		srv.applyDefaults()
		if srv.ID == "" {
			WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("id is required"))
			return
		}
		if err := srv.validate(); err != nil {
			WriteAPIError(w, http.StatusBadRequest, err)
			return
		}
		// Check for duplicates
		for _, existing := range s.cfg.Servers {
			if existing.ID == srv.ID {
				WriteAPIError(w, http.StatusConflict, fmt.Errorf("server %q already exists", srv.ID))
				return
			}
		}
		s.cfg.Servers = append(s.cfg.Servers, srv)

		// Auto-create SSH health check for the new server (CPU, memory, disk, load, uptime, IOPS)
		autoCheck := s.buildAutoSSHCheck(srv)
		autoCheck.applyDefaults()
		if err := s.store.UpsertCheck(autoCheck); err != nil {
			if s.logger != nil {
				s.logger.Printf("Warning: failed to auto-create SSH check for server %q: %v", srv.ID, err)
			}
		} else {
			s.scheduler.UpsertSchedule(autoCheck)
			if s.logger != nil {
				s.logger.Printf("Auto-created SSH health check %q for server %q", autoCheck.ID, srv.ID)
			}
		}

		if s.auditLogger != nil {
			actor := ExtractActorFromRequest(r, s.cfg)
			_ = s.auditLogger.Log("server.created", actor, "server", srv.ID, map[string]interface{}{
				"name": srv.Name,
				"host": srv.Host,
			})
		}

		WriteAPIResponse(w, http.StatusCreated, NewAPIResponse(sanitizeServerForAPI(srv)))

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Service) handleServerByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/servers/")
	if path == "" {
		WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("missing server id"))
		return
	}

	// Handle /api/v1/servers/{id}/live (SSE)
	if strings.HasSuffix(path, "/live") {
		s.handleServerLive(w, r)
		return
	}

	// Handle /api/v1/servers/{id}/test
	if strings.HasSuffix(path, "/test") {
		s.handleServerTest(w, r)
		return
	}

	// Handle /api/v1/servers/{id}/metrics/history
	if strings.Contains(path, "/metrics/history") {
		s.handleServerMetricsHistory(w, r)
		return
	}

	// Handle /api/v1/servers/{id}/metrics
	if strings.HasSuffix(path, "/metrics") {
		s.handleServerMetrics(w, r)
		return
	}

	// Handle /api/v1/servers/{id}/processes
	if strings.HasSuffix(path, "/processes") {
		s.handleServerProcesses(w, r)
		return
	}

	id := path

	switch r.Method {
	case http.MethodGet:
		for _, srv := range s.cfg.Servers {
			if srv.ID == id {
				WriteAPIResponse(w, http.StatusOK, NewAPIResponse(sanitizeServerForAPI(srv)))
				return
			}
		}
		WriteAPIError(w, http.StatusNotFound, fmt.Errorf("server %q not found", id))

	case http.MethodPut, http.MethodPatch:
		if !IsRequestAuthorized(s.cfg.Auth, r) {
			RequestAuth(w)
			return
		}
		var srv RemoteServer
		if err := json.NewDecoder(r.Body).Decode(&srv); err != nil {
			WriteAPIError(w, http.StatusBadRequest, err)
			return
		}
		srv.ID = id
		srv.applyDefaults()
		if err := srv.validate(); err != nil {
			WriteAPIError(w, http.StatusBadRequest, err)
			return
		}

		found := false
		for i, existing := range s.cfg.Servers {
			if existing.ID == id {
				// Preserve password if masked value was sent back
				if srv.Password == "********" {
					srv.Password = existing.Password
				}
				s.cfg.Servers[i] = srv
				found = true
				break
			}
		}
		if !found {
			WriteAPIError(w, http.StatusNotFound, fmt.Errorf("server %q not found", id))
			return
		}

		if s.auditLogger != nil {
			actor := ExtractActorFromRequest(r, s.cfg)
			_ = s.auditLogger.Log("server.updated", actor, "server", id, map[string]interface{}{
				"name": srv.Name,
				"host": srv.Host,
			})
		}

		WriteAPIResponse(w, http.StatusOK, NewAPIResponse(sanitizeServerForAPI(srv)))

	case http.MethodDelete:
		if !IsRequestAuthorized(s.cfg.Auth, r) {
			RequestAuth(w)
			return
		}

		// Check if any checks reference this server
		for _, check := range s.cfg.Checks {
			if check.ServerId == id {
				WriteAPIError(w, http.StatusConflict, fmt.Errorf("cannot delete server %q: check %q references it", id, check.ID))
				return
			}
		}

		found := false
		filtered := make([]RemoteServer, 0, len(s.cfg.Servers))
		for _, srv := range s.cfg.Servers {
			if srv.ID == id {
				found = true
				continue
			}
			filtered = append(filtered, srv)
		}
		if !found {
			WriteAPIError(w, http.StatusNotFound, fmt.Errorf("server %q not found", id))
			return
		}
		s.cfg.Servers = filtered

		if s.auditLogger != nil {
			actor := ExtractActorFromRequest(r, s.cfg)
			_ = s.auditLogger.Log("server.deleted", actor, "server", id, nil)
		}

		w.WriteHeader(http.StatusNoContent)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// handleServerTest tests SSH connectivity to a server.
func (s *Service) handleServerTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !IsRequestAuthorized(s.cfg.Auth, r) {
		RequestAuth(w)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/servers/")
	id = strings.TrimSuffix(id, "/test")
	if id == "" {
		WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("missing server id"))
		return
	}

	var srv *RemoteServer
	for i := range s.cfg.Servers {
		if s.cfg.Servers[i].ID == id {
			srv = &s.cfg.Servers[i]
			break
		}
	}
	if srv == nil {
		WriteAPIError(w, http.StatusNotFound, fmt.Errorf("server %q not found", id))
		return
	}

	output, err := sshDialAndRun(srv.ToSSHConfig(), "echo 'SSH OK' && hostname", 10*time.Second)
	if err != nil {
		WriteAPIResponse(w, http.StatusOK, NewAPIResponse(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		}))
		return
	}

	WriteAPIResponse(w, http.StatusOK, NewAPIResponse(map[string]interface{}{
		"success": true,
		"output":  strings.TrimSpace(string(output)),
	}))
}

// buildAutoSSHCheck creates a default SSH health check for a newly added server.
// Collects all system metrics: CPU, memory, disk, load, uptime, IOPS.
func (s *Service) buildAutoSSHCheck(srv RemoteServer) CheckConfig {
	enabled := true
	return CheckConfig{
		ID:             "ssh-" + srv.ID,
		Name:           srv.Name + " System Health",
		Type:           "ssh",
		Server:         srv.Name,
		ServerId:       srv.ID,
		TimeoutSeconds: 15,
		Enabled:        &enabled,
		Tags:           srv.Tags,
		SSH: &SSHCheckConfig{
			Host:        srv.Host,
			Port:        srv.Port,
			User:        srv.User,
			KeyPath:     srv.KeyPath,
			KeyEnv:      srv.KeyEnv,
			Password:    srv.Password,
			PasswordEnv: srv.PasswordEnv,
			Metrics:     []string{}, // empty = collect all metrics
		},
	}
}

// handleServerMetrics returns the latest metrics snapshot for a server.
func (s *Service) handleServerMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/servers/")
	id := strings.TrimSuffix(path, "/metrics")

	if s.serverMetricsRepo == nil {
		WriteAPIError(w, http.StatusServiceUnavailable, fmt.Errorf("server metrics not available"))
		return
	}

	snap, err := s.serverMetricsRepo.GetLatest(id)
	if err != nil {
		WriteAPIError(w, http.StatusInternalServerError, err)
		return
	}
	if snap == nil {
		WriteAPIError(w, http.StatusNotFound, fmt.Errorf("no metrics available for server %q", id))
		return
	}

	WriteAPIResponse(w, http.StatusOK, NewAPIResponse(snap))
}

// handleServerProcesses returns the latest top processes for a server.
func (s *Service) handleServerProcesses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/servers/")
	id := strings.TrimSuffix(path, "/processes")

	if s.serverMetricsRepo == nil {
		WriteAPIError(w, http.StatusServiceUnavailable, fmt.Errorf("server metrics not available"))
		return
	}

	snap, err := s.serverMetricsRepo.GetLatest(id)
	if err != nil {
		WriteAPIError(w, http.StatusInternalServerError, err)
		return
	}
	if snap == nil {
		WriteAPIError(w, http.StatusNotFound, fmt.Errorf("no process data available for server %q", id))
		return
	}

	WriteAPIResponse(w, http.StatusOK, NewAPIResponse(snap.TopProcesses))
}

// handleServerMetricsHistory returns time-series metrics for charts.
func (s *Service) handleServerMetricsHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/servers/")
	id := strings.Split(path, "/")[0]

	if s.serverMetricsRepo == nil {
		WriteAPIError(w, http.StatusServiceUnavailable, fmt.Errorf("server metrics not available"))
		return
	}

	// Parse time range from query params (default: last 24 hours)
	rangeStr := r.URL.Query().Get("range")
	since := time.Now().Add(-24 * time.Hour)
	switch rangeStr {
	case "1h":
		since = time.Now().Add(-1 * time.Hour)
	case "6h":
		since = time.Now().Add(-6 * time.Hour)
	case "12h":
		since = time.Now().Add(-12 * time.Hour)
	case "24h":
		since = time.Now().Add(-24 * time.Hour)
	case "7d":
		since = time.Now().Add(-7 * 24 * time.Hour)
	case "30d":
		since = time.Now().Add(-30 * 24 * time.Hour)
	}

	snaps, err := s.serverMetricsRepo.GetSnapshots(id, since, time.Time{})
	if err != nil {
		WriteAPIError(w, http.StatusInternalServerError, err)
		return
	}

	// Build time-series response (strip top processes to reduce payload)
	type MetricsPoint struct {
		Timestamp     time.Time `json:"timestamp"`
		CPUPercent    float64   `json:"cpuPercent"`
		MemoryPercent float64   `json:"memoryPercent"`
		MemoryUsedMB  float64   `json:"memoryUsedMB"`
		DiskPercent   float64   `json:"diskPercent"`
		LoadAvg1      float64   `json:"loadAvg1"`
	}

	points := make([]MetricsPoint, 0, len(snaps))
	for _, s := range snaps {
		points = append(points, MetricsPoint{
			Timestamp:     s.Timestamp,
			CPUPercent:    s.CPUUsagePercent,
			MemoryPercent: s.MemoryUsagePercent,
			MemoryUsedMB:  s.MemoryUsedMB,
			DiskPercent:   s.DiskUsagePercent,
			LoadAvg1:      s.LoadAvg1,
		})
	}

	WriteAPIResponse(w, http.StatusOK, NewAPIResponse(points))
}
