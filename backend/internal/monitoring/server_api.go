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

	// Handle /api/v1/servers/{id}/test
	if strings.HasSuffix(path, "/test") {
		s.handleServerTest(w, r)
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
