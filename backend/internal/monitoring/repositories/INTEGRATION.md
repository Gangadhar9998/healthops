# Degraded Mode Integration Guide

## Overview

The degraded mode system provides database-agnostic fail-closed behavior for HealthOps. When the database becomes unavailable, the system:

1. Blocks write operations (POST/PUT/DELETE/PATCH) on protected endpoints
2. Allows read operations (GET/HEAD/OPTIONS) to continue
3. Creates incidents for database degradation
4. Returns 503 Service Unavailable for blocked writes
5. Provides status endpoint for UI banner

## Architecture

```
┌─────────────────┐
│  HTTP Request   │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Degraded Mode  │ ◄── IsDegraded()?
│   Middleware    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Route Handler  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Repository     │
│  (MongoDB/etc)  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│  Health Check   │ ◄── MarkDegraded() / MarkHealthy()
│   Background    │
└─────────────────┘
```

## Integration Steps

### 1. Create Degraded Mode Instance

In `service.go`, add the degraded mode field to the Service struct:

```go
type Service struct {
    // ... existing fields ...
    degradedMode *repositories.DegradedMode
}
```

### 2. Initialize in NewService

```go
import (
    "github.com/medics-health-check/backend/internal/monitoring/repositories"
)

func NewService(cfg *Config, store Store, logger *log.Logger) *Service {
    // ... existing initialization ...

    // Create database health check function
    dbHealthCheck := func(ctx context.Context) error {
        if store.MongoMirror != nil {
            // Test MongoDB connection with timeout
            ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
            defer cancel()
            return store.MongoMirror.Ping(ctx)
        }
        // No database configured, always healthy
        return nil
    }

    // Initialize degraded mode
    svc.degradedMode = repositories.NewDegradedMode(
        dbHealthCheck,
        svc.incidentManager, // Can be nil if not using incidents
        logger,
    )

    // Start health checks in background
    go svc.degradedMode.StartHealthCheck(context.Background(), 30*time.Second)

    return svc
}
```

### 3. Apply Middleware to Routes

In `RegisterRoutes`, apply the middleware:

```go
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
    // ... existing route registration ...

    // Apply degraded mode middleware AFTER other middlewares
    // but BEFORE the HTTP server starts
}
```

Update the middleware chain in `Start`:

```go
// Apply middlewares: degraded mode, then body limit, then rate limit, etc.
var handler http.Handler = mux

// Apply degraded mode FIRST to block writes before other processing
if s.degradedMode != nil {
    // Create a wrapped middleware that only applies to protected endpoints
    handler = degradedModeProtectedMiddleware(s.degradedMode, handler)
}

handler = maxBodyMiddleware(1<<20, handler)
handler = rateLimitMiddleware(100, time.Minute, handler)
// ... rest of middlewares ...
```

Add the helper function:

```go
func degradedModeProtectedMiddleware(dm *repositories.DegradedMode, next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Only check protected endpoints
        if repositories.IsProtectedEndpoint(r.URL.Path) {
            // Check if it's a write operation
            if r.Method == http.MethodPost || r.Method == http.MethodPut ||
               r.Method == http.MethodDelete || r.Method == http.MethodPatch {
                if dm.IsDegraded() {
                    status := dm.GetStatus()
                    respondDegraded(w, status)
                    return
                }
            }
        }
        next.ServeHTTP(w, r)
    })
}

func respondDegraded(w http.ResponseWriter, status repositories.DegradedStatus) {
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
    json.NewEncoder(w).Encode(response)
}
```

### 4. Add System Status Endpoint

Register the system status endpoint:

```go
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
    // ... existing routes ...

    // System status endpoint
    mux.HandleFunc("/api/v1/system/status", s.degradedMode.HandleSystemStatus())
}
```

### 5. Update Readiness Check

Modify the `/readyz` handler to include database status:

```go
func (s *Service) handleReadyz(w http.ResponseWriter, r *http.Request) {
    checks := map[string]interface{}{
        "status": "healthy",
        "checks": map[string]interface{}{
            "database": map[string]interface{}{
                "status": "up",
            },
        },
    }

    if s.degradedMode != nil && s.degradedMode.IsDegraded() {
        status := s.degradedMode.GetStatus()
        checks["status"] = "unhealthy"
        checks["checks"].(map[string]interface{})["database"] = map[string]interface{}{
            "status": "down",
            "error":  status.LastError,
        }

        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusServiceUnavailable)
        json.NewEncoder(w).Encode(checks)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(checks)
}
```

### 6. Add Shutdown Hook

Ensure degraded mode stops gracefully:

```go
func (s *Service) Shutdown(ctx context.Context) error {
    // Stop degraded mode health checks
    if s.degradedMode != nil {
        s.degradedMode.Stop()
    }

    // ... existing shutdown logic ...
}
```

## Protected Endpoints

The following endpoints are protected (writes blocked when degraded):

- `/api/v1/servers` - Server configuration
- `/api/v1/users` - User management
- `/api/v1/notification-channels` - Notification settings
- `/api/v1/alert-rules` - Alert rule configuration
- `/api/v1/ai/config` - AI configuration

These endpoints can be customized by modifying `ProtectedEndpoints()` in `degraded_mode.go`.

## API Response Examples

### Healthy Status

```json
{
  "success": true,
  "data": {
    "healthy": true
  }
}
```

### Degraded Status

```json
{
  "success": false,
  "data": {
    "healthy": false,
    "degradedSince": "2024-04-21T10:30:00Z",
    "lastError": "connection timeout"
  }
}
```

### Blocked Write Response

```json
{
  "success": false,
  "error": {
    "code": 503,
    "message": "System is currently in degraded mode. Database connectivity is unavailable. Read-only operations are permitted.",
    "degraded": {
      "healthy": false,
      "degradedSince": "2024-04-21T10:30:00Z",
      "lastError": "connection timeout"
    }
  }
}
```

### Readiness Check (Degraded)

```json
{
  "status": "unhealthy",
  "checks": {
    "database": {
      "status": "down",
      "error": "connection timeout"
    }
  }
}
```

## Frontend Integration

The frontend should:

1. Poll `/api/v1/system/status` every 30 seconds
2. Show a banner when `healthy: false`
3. Disable write buttons in degraded mode
4. Display `lastError` to users
5. Show `degradedSince` timestamp

Example React integration:

```typescript
interface SystemStatus {
  healthy: boolean;
  degradedSince?: string;
  lastError?: string;
}

function useSystemStatus() {
  const [status, setStatus] = useState<SystemStatus>({ healthy: true });

  useEffect(() => {
    const poll = setInterval(async () => {
      const res = await fetch('/api/v1/system/status');
      const data = await res.json();
      setStatus(data.data);
    }, 30000);

    return () => clearInterval(poll);
  }, []);

  return status;
}

function App() {
  const status = useSystemStatus();

  return (
    <div>
      {!status.healthy && (
        <div className="banner warning">
          ⚠️ System in degraded mode since {new Date(status.degradedSince).toLocaleString()}
          {status.lastError && `: ${status.lastError}`}
        </div>
      )}
      {/* ... rest of app ... */}
    </div>
  );
}
```

## Testing

### Unit Tests

Run the degraded mode tests:

```bash
cd backend
go test ./internal/monitoring/repositories -run TestDegradedMode -v
```

### Integration Tests

Test the middleware:

```bash
# Start service with degraded mode
curl -X POST http://localhost:8080/api/v1/servers -d '{"name":"test"}'
# Should return 503 when degraded

# Check system status
curl http://localhost:8080/api/v1/system/status
# Should show degraded state

# Read operations still work
curl http://localhost:8080/api/v1/checks
# Should return 200 even when degraded
```

### Manual Testing

Simulate database failure:

1. Start the service
2. Block MongoDB connection: `iptables -A OUTPUT -p tcp --dport 27017 -j DROP`
3. Wait for health check to fail (30s)
4. Try POST request - should get 503
5. Try GET request - should work
6. Check `/api/v1/system/status` - should show degraded
7. Restore MongoDB: `iptables -D OUTPUT -p tcp --dport 27017 -j DROP`
8. Wait for health check to pass (30s)
9. POST requests should work again

## Configuration

Environment variables (optional):

- `DEGRADED_MODE_HEALTH_CHECK_INTERVAL` - Health check interval (default: 30s)
- `DEGRADED_MODE_HEALTH_CHECK_TIMEOUT` - Health check timeout (default: 5s)
- `DEGRADED_MODE_RETRY_AFTER` - Retry-After header value (default: 30s)

## Monitoring

Monitor these metrics:

- `degraded_mode_transitions` - Counter for degraded/healthy transitions
- `degraded_mode_duration_seconds` - Histogram for time spent degraded
- `degraded_mode_blocked_requests` - Counter for blocked write requests

These can be added to the Prometheus metrics collector:

```go
// In metrics.go
var (
    degradedModeTransitions = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "healthops_degraded_mode_transitions_total",
            Help: "Number of degraded mode transitions",
        },
        []string{"to"},
    )
    degradedModeDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "healthops_degraded_mode_duration_seconds",
            Help: "Time spent in degraded mode",
        },
        []string{},
    )
)
```

## Troubleshooting

### Health Check Fails Immediately

- Check MongoDB connection string
- Verify network connectivity
- Check MongoDB credentials
- Review firewall rules

### Writes Not Blocked

- Verify middleware is applied correctly
- Check endpoint is in `ProtectedEndpoints()`
- Ensure request method is POST/PUT/DELETE/PATCH

### Incident Not Created

- Verify incident manager is initialized
- Check incident manager logs
- Ensure incident manager is not nil

## Future Enhancements

1. **Automatic Recovery**: Auto-resolve incidents when database recovers
2. **Circuit Breaker**: Add circuit breaker pattern for faster failover
3. **Metrics Integration**: Add Prometheus metrics for degraded mode
4. **Multiple Databases**: Track health of multiple databases independently
5. **Graceful Degradation**: Serve cached/stale data when degraded
6. **Custom Thresholds**: Configure consecutive failures before marking degraded
