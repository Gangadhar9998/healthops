package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// DegradedStatus represents the current degraded mode state
type DegradedStatus struct {
	Healthy       bool      `json:"healthy"`
	DegradedSince time.Time `json:"degradedSince,omitempty"`
	LastError     string    `json:"lastError,omitempty"`
}

// DegradedMode provides fail-closed behavior when database is unavailable
type DegradedMode struct {
	healthCheck       func(ctx context.Context) error
	incidentMgr       incidentManager
	logger            *log.Logger
	mu                sync.RWMutex
	status            DegradedStatus
	cancelHealthCheck context.CancelFunc
	stopped           chan struct{}
}

// incidentManager defines the interface for creating degradation incidents
type incidentManager interface {
	ProcessAlert(checkID, checkName, checkType, severity, message string, metadata map[string]string) error
}

// NewDegradedMode creates a new degraded mode instance
func NewDegradedMode(
	healthCheck func(ctx context.Context) error,
	incidentMgr incidentManager,
	logger *log.Logger,
) *DegradedMode {
	return &DegradedMode{
		healthCheck: healthCheck,
		incidentMgr: incidentMgr,
		logger:      logger,
		status: DegradedStatus{
			Healthy: true,
		},
		stopped: make(chan struct{}),
	}
}

// StartHealthCheck begins periodic health checks
func (dm *DegradedMode) StartHealthCheck(ctx context.Context, interval time.Duration) {
	ctx, cancel := context.WithCancel(ctx)
	dm.cancelHealthCheck = cancel

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run initial health check
	dm.runHealthCheck(ctx)

	for {
		select {
		case <-ticker.C:
			dm.runHealthCheck(ctx)
		case <-ctx.Done():
			close(dm.stopped)
			return
		case <-dm.stopped:
			return
		}
	}
}

// Stop halts the health check goroutine
func (dm *DegradedMode) Stop() {
	if dm.cancelHealthCheck != nil {
		dm.cancelHealthCheck()
	}
	// Wait for goroutine to stop
	select {
	case <-dm.stopped:
	case <-time.After(5 * time.Second):
		if dm.logger != nil {
			dm.logger.Printf("WARNING: DegradedMode health check did not stop gracefully")
		}
	}
}

// SetIncidentManager sets the incident manager (used for delayed initialization)
func (dm *DegradedMode) SetIncidentManager(im incidentManager) {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	dm.incidentMgr = im
}

// runHealthCheck executes a single health check and updates status
func (dm *DegradedMode) runHealthCheck(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := dm.healthCheck(checkCtx)

	dm.mu.Lock()
	defer dm.mu.Unlock()

	wasHealthy := dm.status.Healthy
	now := time.Now().UTC()

	if err != nil {
		// Health check failed
		if wasHealthy {
			// Transition to degraded
			dm.status.Healthy = false
			dm.status.DegradedSince = now
			dm.status.LastError = err.Error()

			if dm.logger != nil {
				dm.logger.Printf("DEGRADED MODE: Database unavailable: %v", err)
			}

			// Create incident if manager is available
			if dm.incidentMgr != nil {
				metadata := map[string]string{
					"degradedSince": dm.status.DegradedSince.Format(time.RFC3339),
					"error":         dm.status.LastError,
				}
				_ = dm.incidentMgr.ProcessAlert(
					"database",
					"Database Connectivity",
					"system",
					"critical",
					fmt.Sprintf("Database unavailable: %v", err),
					metadata,
				)
			}
		}
	} else {
		// Health check passed
		if !wasHealthy {
			// Transition back to healthy
			dm.status.Healthy = true
			duration := now.Sub(dm.status.DegradedSince)
			dm.status.DegradedSince = time.Time{}
			dm.status.LastError = ""

			if dm.logger != nil {
				dm.logger.Printf("DEGRADED MODE: Database restored after %v", duration.Round(time.Second))
			}

			// Auto-resolve incident if manager is available
			if dm.incidentMgr != nil {
				// Note: IncidentManager doesn't have a public resolve method
				// The incident will be auto-resolved on next health check cycle
				// via AutoResolveOnRecovery in the scheduler
			}
		}
	}
}

// IsDegraded returns true if the system is currently in degraded mode
func (dm *DegradedMode) IsDegraded() bool {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return !dm.status.Healthy
}

// GetStatus returns the current degraded status
func (dm *DegradedMode) GetStatus() DegradedStatus {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.status
}

// Middleware returns an HTTP middleware that blocks writes when degraded
func (dm *DegradedMode) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only check protected endpoints
		if !isProtectedEndpoint(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Only block write operations
		if !isWriteMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		// Check if degraded
		if dm.IsDegraded() {
			status := dm.GetStatus()
			respondDegraded(w, status)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// isProtectedEndpoint returns true if the endpoint should be protected
func isProtectedEndpoint(path string) bool {
	protectedPaths := []string{
		"/api/v1/checks",
		"/api/v1/config",
		"/api/v1/servers",
		"/api/v1/users",
		"/api/v1/notification-channels",
		"/api/v1/alert-rules",
		"/api/v1/ai/config",
	}

	for _, prefix := range protectedPaths {
		if path == prefix || (len(path) > len(prefix) && path[:len(prefix)+1] == prefix+"/") {
			return true
		}
	}

	return false
}

// isWriteMethod returns true if the HTTP method is a write operation
func isWriteMethod(method string) bool {
	return method == http.MethodPost || method == http.MethodPut ||
		method == http.MethodDelete || method == http.MethodPatch
}

// respondDegraded sends a 503 Service Unavailable response
func respondDegraded(w http.ResponseWriter, status DegradedStatus) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "30")
	w.WriteHeader(http.StatusServiceUnavailable)

	response := map[string]interface{}{
		"success": false,
		"error": map[string]interface{}{
			"code":    http.StatusServiceUnavailable,
			"message": "System is in degraded mode. Database unavailable. Read-only mode.",
		},
	}
	_ = json.NewEncoder(w).Encode(response)
}

// HandleSystemStatus returns an HTTP handler for the system status endpoint
func (dm *DegradedMode) HandleSystemStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		status := dm.GetStatus()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data":    status,
		})
	}
}
