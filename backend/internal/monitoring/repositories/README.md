# Degraded Mode System

A database-agnostic degraded mode implementation for HealthOps monitoring system.

## Features

- **Database Health Tracking**: Monitors database connectivity with configurable health checks
- **Write Blocking**: Automatically blocks POST/PUT/DELETE/PATCH operations when database is down
- **Read-Only Mode**: Allows GET/HEAD/OPTIONS requests to continue during degradation
- **Incident Integration**: Automatically creates incidents when entering degraded mode
- **Status API**: Provides `/api/v1/system/status` endpoint for UI integration
- **Readiness Check**: Updates `/readyz` to reflect database health

## Files

- `degraded_mode.go` - Core degraded mode implementation
- `degraded_mode_test.go` - Comprehensive unit tests
- `example_test.go` - Usage examples
- `INTEGRATION.md` - Complete integration guide

## Quick Start

```go
import "github.com/medics-health-check/backend/internal/monitoring/repositories"

// Create health check function
healthCheck := func(ctx context.Context) error {
    return db.PingContext(ctx)
}

// Initialize degraded mode
dm := repositories.NewDegradedMode(healthCheck, incidentManager, logger)

// Start health checks
go dm.StartHealthCheck(context.Background(), 30*time.Second)

// Apply middleware
handler := dm.Middleware(http.DefaultServeMux)

// Add status endpoint
http.HandleFunc("/api/v1/system/status", dm.HandleSystemStatus())
```

## API Endpoints

### GET /api/v1/system/status

Returns current system status:

```json
{
  "success": true,
  "data": {
    "healthy": false,
    "degradedSince": "2024-04-21T10:30:00Z",
    "lastError": "connection timeout"
  }
}
```

### Protected Endpoints (writes blocked when degraded)

- `/api/v1/servers`
- `/api/v1/users`
- `/api/v1/notification-channels`
- `/api/v1/alert-rules`
- `/api/v1/ai/config`

## Behavior

### When Degraded

- POST/PUT/DELETE/PATCH requests to protected endpoints return **503 Service Unavailable**
- GET/HEAD/OPTIONS requests continue normally
- Response includes `Retry-After: 30` header
- Incident is created automatically
- `/readyz` returns **503** with database status

### When Healthy

- All requests processed normally
- Status endpoint returns **200 OK**
- `/readyz` returns **200 OK**

## Testing

```bash
# Run tests
go test ./internal/monitoring/repositories -run TestDegradedMode -v

# Run example tests
go test ./internal/monitoring/repositories -run Example
```

## Configuration

Environment variables (optional):

- `DEGRADED_MODE_HEALTH_CHECK_INTERVAL` - Health check interval (default: 30s)
- `DEGRADED_MODE_HEALTH_CHECK_TIMEOUT` - Health check timeout (default: 5s)

## Thread Safety

All methods are thread-safe and use `sync.RWMutex` for concurrent access.

## Integration

See [INTEGRATION.md](./INTEGRATION.md) for complete integration guide with examples.

## License

Part of the HealthOps monitoring system.
