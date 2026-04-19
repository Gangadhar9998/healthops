package monitoring

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// MySQLAPIHandler holds dependencies for MySQL-related API endpoints.
type MySQLAPIHandler struct {
	mysqlRepo    MySQLMetricsRepository
	snapshotRepo IncidentSnapshotRepository
	outbox       NotificationOutboxRepository
	aiQueue      *FileAIQueue
	auditLogger  *AuditLogger
	cfg          *Config
}

// NewMySQLAPIHandler creates a new MySQL API handler.
func NewMySQLAPIHandler(
	mysqlRepo MySQLMetricsRepository,
	snapshotRepo IncidentSnapshotRepository,
	outbox NotificationOutboxRepository,
	aiQueue *FileAIQueue,
	auditLogger *AuditLogger,
	cfg *Config,
) *MySQLAPIHandler {
	return &MySQLAPIHandler{
		mysqlRepo:    mysqlRepo,
		snapshotRepo: snapshotRepo,
		outbox:       outbox,
		aiQueue:      aiQueue,
		auditLogger:  auditLogger,
		cfg:          cfg,
	}
}

// RegisterRoutes registers MySQL-related API routes on the given mux.
func (h *MySQLAPIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/mysql/samples", h.handleMySQLSamples)
	mux.HandleFunc("/api/v1/mysql/deltas", h.handleMySQLDeltas)
	mux.HandleFunc("/api/v1/incidents/", h.handleIncidentSnapshots) // extends existing pattern
	mux.HandleFunc("/api/v1/notifications", h.handleNotifications)
	mux.HandleFunc("/api/v1/notifications/", h.handleNotificationByID)
	mux.HandleFunc("/api/v1/ai/queue", h.handleAIQueue)
	mux.HandleFunc("/api/v1/ai/queue/", h.handleAIQueueByID)
}

// GET /api/v1/mysql/samples?checkId=...&limit=...
func (h *MySQLAPIHandler) handleMySQLSamples(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	checkID := strings.TrimSpace(r.URL.Query().Get("checkId"))
	if checkID == "" {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("checkId is required"))
		return
	}

	limit := queryInt(r, "limit", 20)
	samples, err := h.mysqlRepo.RecentSamples(checkID, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}

	writeAPIResponse(w, http.StatusOK, NewAPIResponse(samples))
}

// GET /api/v1/mysql/deltas?checkId=...&limit=...
func (h *MySQLAPIHandler) handleMySQLDeltas(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	checkID := strings.TrimSpace(r.URL.Query().Get("checkId"))
	if checkID == "" {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("checkId is required"))
		return
	}

	limit := queryInt(r, "limit", 20)
	deltas, err := h.mysqlRepo.RecentDeltas(checkID, limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}

	writeAPIResponse(w, http.StatusOK, NewAPIResponse(deltas))
}

// GET /api/v1/incidents/{id}/snapshots
func (h *MySQLAPIHandler) handleIncidentSnapshots(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		// Let other handlers deal with non-snapshot incident routes
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/incidents/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "snapshots" {
		return // not a snapshots request, let service.go handle it
	}

	incidentID := parts[0]
	if incidentID == "" {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("missing incident id"))
		return
	}

	snaps, err := h.snapshotRepo.GetSnapshots(incidentID)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}

	writeAPIResponse(w, http.StatusOK, NewAPIResponse(snaps))
}

// GET /api/v1/notifications?status=pending&limit=...
func (h *MySQLAPIHandler) handleNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	limit := queryInt(r, "limit", 100)
	events, err := h.outbox.ListPending(limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}

	writeAPIResponse(w, http.StatusOK, NewAPIResponse(events))
}

// POST /api/v1/notifications/{id}/sent
// POST /api/v1/notifications/{id}/failed
func (h *MySQLAPIHandler) handleNotificationByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if !isRequestAuthorized(h.cfg.Auth, r) {
		requestAuth(w)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/notifications/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("expected /api/v1/notifications/{id}/{action}"))
		return
	}

	notifID := parts[0]
	action := parts[1]

	switch action {
	case "sent":
		if err := h.outbox.MarkSent(notifID); err != nil {
			writeAPIError(w, http.StatusInternalServerError, err)
			return
		}
		if h.auditLogger != nil {
			actor := ExtractActorFromRequest(r, h.cfg)
			_ = h.auditLogger.Log("notification.sent", actor, "notification", notifID, nil)
		}
		writeAPIResponse(w, http.StatusOK, NewAPIResponse(map[string]string{"status": "sent"}))

	case "failed":
		var payload struct {
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		if err := h.outbox.MarkFailed(notifID, payload.Reason); err != nil {
			writeAPIError(w, http.StatusInternalServerError, err)
			return
		}
		if h.auditLogger != nil {
			actor := ExtractActorFromRequest(r, h.cfg)
			_ = h.auditLogger.Log("notification.failed", actor, "notification", notifID, map[string]interface{}{
				"reason": payload.Reason,
			})
		}
		writeAPIResponse(w, http.StatusOK, NewAPIResponse(map[string]string{"status": "failed"}))

	default:
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("unknown action: %s", action))
	}
}

// GET /api/v1/ai/queue?status=pending&limit=...
func (h *MySQLAPIHandler) handleAIQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	limit := queryInt(r, "limit", 100)
	items, err := h.aiQueue.ListPendingItems(limit)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}

	writeAPIResponse(w, http.StatusOK, NewAPIResponse(items))
}

// POST /api/v1/ai/queue/{incidentId}/done
// POST /api/v1/ai/queue/{incidentId}/failed
func (h *MySQLAPIHandler) handleAIQueueByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if !isRequestAuthorized(h.cfg.Auth, r) {
		requestAuth(w)
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/v1/ai/queue/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("expected /api/v1/ai/queue/{incidentId}/{action}"))
		return
	}

	incidentID := parts[0]
	action := parts[1]

	switch action {
	case "done":
		var result AIAnalysisResult
		if err := json.NewDecoder(r.Body).Decode(&result); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		if err := h.aiQueue.Complete(incidentID, result); err != nil {
			writeAPIError(w, http.StatusInternalServerError, err)
			return
		}
		if h.auditLogger != nil {
			actor := ExtractActorFromRequest(r, h.cfg)
			_ = h.auditLogger.Log("ai.analysis.completed", actor, "incident", incidentID, nil)
		}
		writeAPIResponse(w, http.StatusOK, NewAPIResponse(map[string]string{"status": "completed"}))

	case "failed":
		var payload struct {
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeAPIError(w, http.StatusBadRequest, err)
			return
		}
		if err := h.aiQueue.Fail(incidentID, payload.Reason); err != nil {
			writeAPIError(w, http.StatusInternalServerError, err)
			return
		}
		if h.auditLogger != nil {
			actor := ExtractActorFromRequest(r, h.cfg)
			_ = h.auditLogger.Log("ai.analysis.failed", actor, "incident", incidentID, map[string]interface{}{
				"reason": payload.Reason,
			})
		}
		writeAPIResponse(w, http.StatusOK, NewAPIResponse(map[string]string{"status": "failed"}))

	default:
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("unknown action: %s", action))
	}
}
