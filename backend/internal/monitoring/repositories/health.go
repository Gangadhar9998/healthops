package repositories

import (
	"context"
)

// HealthStatus represents the health check result for a repository
type HealthStatus struct {
	Healthy bool   `json:"healthy"` // true if the repository is operational
	Message string `json:"message"` // human-readable status description
	Latency string `json:"latency,omitempty"` // optional latency measurement
}

// HealthChecker provides database health verification
//
// Implementations should:
// - Verify connectivity to the backing store
// - Perform a lightweight operation (ping, simple query)
// - Return detailed error information for troubleshooting
// - Honor context cancellation/timeout
//
// This interface is optional for repositories that don't require health checks
// (e.g., in-memory stores, file-based stores without external dependencies).
type HealthChecker interface {
	// HealthCheck verifies the repository is operational
	HealthCheck(ctx context.Context) (*HealthStatus, error)
}
