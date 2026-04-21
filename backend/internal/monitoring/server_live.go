package monitoring

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// handleServerLive streams ServerSnapshot via SSE for a specific server.
func (s *Service) handleServerLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		WriteAPIError(w, http.StatusInternalServerError, fmt.Errorf("streaming not supported"))
		return
	}

	// Extract server ID from path: /api/v1/servers/{id}/live
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/servers/")
	serverID := strings.TrimSuffix(path, "/live")
	if serverID == "" {
		WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("missing server id"))
		return
	}

	if s.serverMetricsRepo == nil {
		WriteAPIError(w, http.StatusServiceUnavailable, fmt.Errorf("server metrics not available"))
		return
	}

	// Verify server exists
	found := false
	for _, srv := range s.cfg.Servers {
		if srv.ID == serverID {
			found = true
			break
		}
	}
	if !found {
		WriteAPIError(w, http.StatusNotFound, fmt.Errorf("server %q not found", serverID))
		return
	}

	intervalSec := 5
	if qs := r.URL.Query().Get("interval"); qs != "" {
		if v, err := strconv.Atoi(qs); err == nil && v >= 2 && v <= 30 {
			intervalSec = v
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	corsOrigin := os.Getenv("CORS_ORIGIN")
	if corsOrigin != "" {
		w.Header().Set("Access-Control-Allow-Origin", corsOrigin)
	}

	ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
	defer ticker.Stop()

	// Send initial snapshot immediately
	s.sendServerLiveEvent(w, flusher, serverID)

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			s.sendServerLiveEvent(w, flusher, serverID)
		}
	}
}

func (s *Service) sendServerLiveEvent(w http.ResponseWriter, flusher http.Flusher, serverID string) {
	snap, err := s.serverMetricsRepo.GetLatest(serverID)
	if err != nil || snap == nil {
		return
	}

	data, err := json.Marshal(snap)
	if err != nil {
		return
	}

	fmt.Fprintf(w, "event: snapshot\ndata: %s\n\n", data)
	flusher.Flush()
}
