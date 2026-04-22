package notify

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"medics-health-check/backend/internal/monitoring"
)

// NotificationAPIHandler handles notification channel API endpoints.
type NotificationAPIHandler struct {
	channelStore ChannelStore
	dispatcher   *NotificationDispatcher
	cfg          *monitoring.Config
}

// NewNotificationAPIHandler creates a new notification channel API handler.
func NewNotificationAPIHandler(
	channelStore ChannelStore,
	dispatcher *NotificationDispatcher,
	cfg *monitoring.Config,
) *NotificationAPIHandler {
	return &NotificationAPIHandler{
		channelStore: channelStore,
		dispatcher:   dispatcher,
		cfg:          cfg,
	}
}

// RegisterRoutes registers notification channel API routes.
func (h *NotificationAPIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/notification-channels", h.handleChannels)
	mux.HandleFunc("/api/v1/notification-channels/", h.handleChannelByID)
	mux.HandleFunc("/api/v1/notification-channels/test", h.handleTestChannel)
}

// GET  /api/v1/notification-channels — list all channels
// POST /api/v1/notification-channels — create a new channel
func (h *NotificationAPIHandler) handleChannels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		channels := h.channelStore.List()
		monitoring.WriteAPIResponse(w, http.StatusOK, monitoring.NewAPIResponse(channels))

	case http.MethodPost:
		if !monitoring.IsRequestAuthorized(h.cfg.Auth, r) {
			monitoring.RequestAuth(w)
			return
		}

		var ch NotificationChannelConfig
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&ch); err != nil {
			monitoring.WriteAPIError(w, http.StatusBadRequest, err)
			return
		}

		if err := h.channelStore.Create(ch); err != nil {
			monitoring.WriteAPIError(w, http.StatusBadRequest, err)
			return
		}

		created, ok := h.channelStore.Get(ch.ID)
		if !ok {
			monitoring.WriteAPIResponse(w, http.StatusCreated, monitoring.NewAPIResponse(ch.SafeView()))
			return
		}
		monitoring.WriteAPIResponse(w, http.StatusCreated, monitoring.NewAPIResponse(created))

	default:
		monitoring.WriteAPIError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}

// Handles /api/v1/notification-channels/{id} and sub-paths like /toggle
func (h *NotificationAPIHandler) handleChannelByID(w http.ResponseWriter, r *http.Request) {
	// Let the exact /test path be handled by handleTestChannel
	if r.URL.Path == "/api/v1/notification-channels/test" {
		h.handleTestChannel(w, r)
		return
	}

	raw := strings.TrimPrefix(r.URL.Path, "/api/v1/notification-channels/")
	if raw == "" {
		monitoring.WriteAPIError(w, http.StatusBadRequest, fmt.Errorf("missing channel id"))
		return
	}

	// Check for sub-paths: {id}/toggle
	parts := strings.SplitN(raw, "/", 2)
	id := parts[0]
	subPath := ""
	if len(parts) > 1 {
		subPath = parts[1]
	}

	switch subPath {
	case "toggle":
		h.handleToggle(w, r, id)
		return
	case "":
		// fall through to CRUD
	default:
		monitoring.WriteAPIError(w, http.StatusNotFound, fmt.Errorf("unknown path: %s", subPath))
		return
	}

	switch r.Method {
	case http.MethodGet:
		ch, ok := h.channelStore.Get(id)
		if !ok {
			monitoring.WriteAPIError(w, http.StatusNotFound, fmt.Errorf("channel not found: %s", id))
			return
		}
		monitoring.WriteAPIResponse(w, http.StatusOK, monitoring.NewAPIResponse(ch))

	case http.MethodPut:
		if !monitoring.IsRequestAuthorized(h.cfg.Auth, r) {
			monitoring.RequestAuth(w)
			return
		}

		var ch NotificationChannelConfig
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&ch); err != nil {
			monitoring.WriteAPIError(w, http.StatusBadRequest, err)
			return
		}

		if err := h.channelStore.Update(id, ch); err != nil {
			monitoring.WriteAPIError(w, http.StatusBadRequest, err)
			return
		}

		updated, ok := h.channelStore.Get(id)
		if !ok {
			monitoring.WriteAPIError(w, http.StatusInternalServerError, fmt.Errorf("channel not found after update"))
			return
		}
		monitoring.WriteAPIResponse(w, http.StatusOK, monitoring.NewAPIResponse(updated))

	case http.MethodDelete:
		if !monitoring.IsRequestAuthorized(h.cfg.Auth, r) {
			monitoring.RequestAuth(w)
			return
		}

		if err := h.channelStore.Delete(id); err != nil {
			monitoring.WriteAPIError(w, http.StatusNotFound, err)
			return
		}
		monitoring.WriteAPIResponse(w, http.StatusOK, monitoring.NewAPIResponse(map[string]string{"deleted": id}))

	default:
		monitoring.WriteAPIError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}

// POST /api/v1/notification-channels/{id}/toggle — enable/disable a channel
func (h *NotificationAPIHandler) handleToggle(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		monitoring.WriteAPIError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}

	if !monitoring.IsRequestAuthorized(h.cfg.Auth, r) {
		monitoring.RequestAuth(w)
		return
	}

	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		monitoring.WriteAPIError(w, http.StatusBadRequest, err)
		return
	}

	if err := h.channelStore.ToggleEnabled(id, body.Enabled); err != nil {
		monitoring.WriteAPIError(w, http.StatusNotFound, err)
		return
	}

	ch, ok := h.channelStore.Get(id)
	if !ok {
		monitoring.WriteAPIError(w, http.StatusInternalServerError, fmt.Errorf("channel not found after toggle"))
		return
	}
	monitoring.WriteAPIResponse(w, http.StatusOK, monitoring.NewAPIResponse(ch))
}

// POST /api/v1/notification-channels/test — test a channel config without saving
func (h *NotificationAPIHandler) handleTestChannel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		monitoring.WriteAPIError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}

	if !monitoring.IsRequestAuthorized(h.cfg.Auth, r) {
		monitoring.RequestAuth(w)
		return
	}

	var ch NotificationChannelConfig
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&ch); err != nil {
		monitoring.WriteAPIError(w, http.StatusBadRequest, err)
		return
	}

	if err := ch.Validate(); err != nil {
		monitoring.WriteAPIError(w, http.StatusBadRequest, err)
		return
	}

	if err := h.dispatcher.TestChannel(ch); err != nil {
		monitoring.WriteAPIError(w, http.StatusBadGateway, fmt.Errorf("test failed: %w", err))
		return
	}

	monitoring.WriteAPIResponse(w, http.StatusOK, monitoring.NewAPIResponse(map[string]string{
		"status":  "ok",
		"message": "test notification sent successfully",
	}))
}
