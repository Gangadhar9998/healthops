package monitoring

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// APIResponse is the standard envelope for all API responses
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *APIError   `json:"error,omitempty"`
}

// APIError represents an error response
type APIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// PaginatedResponse wraps a paginated list with metadata
type PaginatedResponse struct {
	Items  interface{} `json:"items"`
	Total  int         `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
}

// CheckListItem is a lightweight representation for list views
type CheckListItem struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Server      string   `json:"server,omitempty"`
	Application string   `json:"application,omitempty"`
	Enabled     bool     `json:"enabled"`
	Tags        []string `json:"tags,omitempty"`
}

// NewAPIResponse creates a successful API response
func NewAPIResponse(data interface{}) APIResponse {
	return APIResponse{
		Success: true,
		Data:    data,
	}
}

// NewAPIError creates an API error
func NewAPIError(code int, message string) APIError {
	return APIError{
		Code:    code,
		Message: message,
	}
}

// NewPaginatedResponse creates a paginated API response
func NewPaginatedResponse(items interface{}, total, limit, offset int) APIResponse {
	return APIResponse{
		Success: true,
		Data: PaginatedResponse{
			Items:  items,
			Total:  total,
			Limit:  limit,
			Offset: offset,
		},
	}
}

// toCheckListItem converts a CheckConfig to a CheckListItem
func toCheckListItem(check CheckConfig) CheckListItem {
	return CheckListItem{
		ID:          check.ID,
		Name:        check.Name,
		Type:        check.Type,
		Server:      check.Server,
		Application: check.Application,
		Enabled:     check.IsEnabled(),
		Tags:        cloneTags(check.Tags),
	}
}

// toCheckListItems converts a slice of CheckConfig to CheckListItem
func toCheckListItems(checks []CheckConfig) []CheckListItem {
	items := make([]CheckListItem, len(checks))
	for i := range checks {
		items[i] = toCheckListItem(checks[i])
	}
	return items
}

// sanitizeChecksForList returns full CheckConfig objects with sensitive metadata stripped
func sanitizeChecksForList(checks []CheckConfig) []CheckConfig {
	safe := make([]CheckConfig, len(checks))
	for i := range checks {
		safe[i] = checks[i]
		safe[i].Metadata = nil
	}
	return safe
}

// WriteAPIResponse writes an APIResponse to the response writer
func WriteAPIResponse(w http.ResponseWriter, statusCode int, response APIResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(response)
}

// WriteAPIError writes an error APIResponse
func WriteAPIError(w http.ResponseWriter, statusCode int, err error) {
	WriteAPIResponse(w, statusCode, APIResponse{
		Success: false,
		Error: &APIError{
			Code:    statusCode,
			Message: err.Error(),
		},
	})
}

// QueryInt parses an integer query parameter with a fallback default.
func QueryInt(r *http.Request, key string, fallback int) int {
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
