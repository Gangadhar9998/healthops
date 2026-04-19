package monitoring

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

const (
	maxRequestSize = 1 << 20 // 1MB
	maxNameLength  = 200
	maxIDLength    = 100
)

var (
	checkIDRegex = regexp.MustCompile(`^[a-z0-9-]+$`)
	htmlTagRegex = regexp.MustCompile(`<[^>]*>`)
)

// ValidateCheckID validates that a check ID matches the required pattern.
func ValidateCheckID(id string) error {
	if id == "" {
		return errors.New("check ID cannot be empty")
	}
	if len(id) > maxIDLength {
		return fmt.Errorf("check ID must be %d characters or fewer", maxIDLength)
	}
	if !checkIDRegex.MatchString(id) {
		return fmt.Errorf("check ID must match pattern %q, got %q", checkIDRegex.String(), id)
	}
	return nil
}

// validateName validates a name field for length and dangerous content.
func validateName(name string) error {
	if name == "" {
		return errors.New("name is required")
	}
	if len(name) > maxNameLength {
		return fmt.Errorf("name must be %d characters or fewer", maxNameLength)
	}
	if htmlTagRegex.MatchString(name) {
		return errors.New("name must not contain HTML tags")
	}
	return nil
}

// ValidateAndDecodeCheck reads and validates a check configuration from the request body.
// It enforces request size limits and validates the check ID format.
func ValidateAndDecodeCheck(r io.Reader) (CheckConfig, error) {
	// Limit reader to prevent large payloads
	limitedReader := io.LimitReader(r, maxRequestSize)

	var check CheckConfig
	if err := json.NewDecoder(limitedReader).Decode(&check); err != nil {
		return CheckConfig{}, fmt.Errorf("invalid JSON: %w", err)
	}

	// Validate check ID format
	if check.ID != "" {
		if err := ValidateCheckID(check.ID); err != nil {
			return CheckConfig{}, err
		}
	}

	// Validate required fields
	if err := validateName(check.Name); err != nil {
		return CheckConfig{}, err
	}

	if check.Type == "" {
		return CheckConfig{}, errors.New("type is required")
	}

	return check, nil
}

// QueryIntRange validates and clamps a query parameter to the specified range.
// If the parameter is missing, invalid, or out of range, the fallback value is returned.
func QueryIntRange(r *http.Request, key string, min, max, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}

	if value < min {
		return min
	}
	if value > max {
		return max
	}

	return value
}

// Internal versions for use within the package
func validateCheckID(id string) error {
	return ValidateCheckID(id)
}

func validateAndDecodeCheck(r io.Reader) (CheckConfig, error) {
	return ValidateAndDecodeCheck(r)
}

func queryIntRange(r *http.Request, key string, min, max, fallback int) int {
	return QueryIntRange(r, key, min, max, fallback)
}
