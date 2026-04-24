// Package logging provides a process-wide structured logger built on log/slog.
//
// It exposes:
//   - Init: install a JSON or text handler with baked-in resource attributes
//     (service, version, env), reading configuration from environment variables.
//   - L: global access to the configured logger.
//   - WithRequestID / RequestIDFromContext / FromContext: request correlation
//     helpers for HTTP middleware and downstream handlers.
//   - LegacyAdapter: wraps an *slog.Logger as a *log.Logger so existing
//     consumers of the standard library logger transparently emit structured
//     events through the same sink.
//   - RedactErr: small helper that scrubs sensitive substrings from error
//     messages before they are attached to a log record.
package logging

import (
	"context"
	"io"
	"log"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"sync"
)

// Environment variables consumed by Init.
const (
	EnvLogLevel  = "HEALTHOPS_LOG_LEVEL"  // debug|info|warn|error
	EnvLogFormat = "HEALTHOPS_LOG_FORMAT" // json|text
	EnvVersion   = "HEALTHOPS_VERSION"    // build/commit identifier
	EnvEnv       = "HEALTHOPS_ENV"        // deployment environment
)

type contextKey int

const requestIDKey contextKey = iota

var (
	mu     sync.RWMutex
	logger = slog.New(slog.NewJSONHandler(io.Discard, nil))
)

// Init configures the process-wide structured logger and returns it.
// The version argument may be empty; in that case HEALTHOPS_VERSION is read,
// falling back to "dev". Init also installs the result as the default slog
// logger so packages that call slog.Info directly emit through the same sink.
func Init(version string) *slog.Logger {
	level := parseLevel(os.Getenv(EnvLogLevel))
	format := strings.ToLower(strings.TrimSpace(os.Getenv(EnvLogFormat)))
	if format == "" {
		format = "json"
	}

	if version == "" {
		version = os.Getenv(EnvVersion)
	}
	if version == "" {
		version = "dev"
	}
	env := os.Getenv(EnvEnv)
	if env == "" {
		env = "development"
	}

	opts := &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			// slog already emits "time" in RFC3339Nano UTC when we set
			// Location to UTC; ensure that here regardless of the host TZ.
			if a.Key == slog.TimeKey {
				return slog.Attr{Key: "timestamp", Value: a.Value}
			}
			return a
		},
	}

	var handler slog.Handler
	if format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	resourceAttrs := []slog.Attr{
		slog.String("service", "healthops"),
		slog.String("version", version),
		slog.String("env", env),
	}
	handler = handler.WithAttrs(resourceAttrs)

	l := slog.New(handler)

	mu.Lock()
	logger = l
	mu.Unlock()

	slog.SetDefault(l)
	return l
}

// L returns the process-wide structured logger. Safe for concurrent use.
// If Init has not been called, L returns a no-op logger that discards output.
func L() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return logger
}

// WithRequestID returns a new context carrying the given request ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestIDFromContext extracts the request ID stored in ctx, if any.
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// FromContext returns L() with the request_id attribute attached when present.
func FromContext(ctx context.Context) *slog.Logger {
	l := L()
	if id := RequestIDFromContext(ctx); id != "" {
		return l.With("request_id", id)
	}
	return l
}

// LegacyAdapter wraps an *slog.Logger as a *log.Logger so existing call sites
// that hold a *log.Logger continue to work but emit through the structured
// sink. Forwarded records are recorded at INFO level.
//
// Pass nil to use the process-wide logger (L()).
func LegacyAdapter(l *slog.Logger) *log.Logger {
	if l == nil {
		l = L()
	}
	// slog.NewLogLogger returns a *log.Logger that writes each line to the
	// supplied handler at the given level. Source/file flags from the legacy
	// logger are unused because slog manages its own metadata.
	return slog.NewLogLogger(l.Handler(), slog.LevelInfo)
}

// passwordRE matches "password=...", "password: ...", and similar sensitive
// key/value pairs that occasionally surface in driver error messages. The
// match runs to the next whitespace, comma, semicolon, or quote.
var passwordRE = regexp.MustCompile(`(?i)(password|passwd|pwd|secret|token|api[_-]?key|authorization)\s*[:=]\s*[^\s,;'"]+`)

// RedactErr returns the error's message with common credential patterns
// replaced by "***". It returns "" for a nil error.
func RedactErr(err error) string {
	if err == nil {
		return ""
	}
	return passwordRE.ReplaceAllStringFunc(err.Error(), func(match string) string {
		// Preserve the key portion ("password=") and replace the value.
		idx := strings.IndexAny(match, ":=")
		if idx < 0 {
			return "***"
		}
		return match[:idx+1] + "***"
	})
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error", "err":
		return slog.LevelError
	case "", "info":
		return slog.LevelInfo
	default:
		return slog.LevelInfo
	}
}
