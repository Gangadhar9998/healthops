package monitoring

import (
	"encoding/csv"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ExportFormat type for data export.
type ExportFormat string

const (
	ExportCSV  ExportFormat = "csv"
	ExportJSON ExportFormat = "json"
)

// handleExportIncidents returns an http.HandlerFunc that exports incidents as CSV or JSON.
func handleExportIncidents(incidentRepo IncidentRepository) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		format := ExportFormat(strings.TrimSpace(r.URL.Query().Get("format")))
		if format == "" {
			format = ExportJSON
		}

		incidents, err := incidentRepo.ListIncidents()
		if err != nil {
			WriteAPIError(w, http.StatusInternalServerError, err)
			return
		}

		if format == ExportCSV {
			w.Header().Set("Content-Type", "text/csv")
			w.Header().Set("Content-Disposition", "attachment; filename=incidents.csv")

			cw := csv.NewWriter(w)
			_ = cw.Write([]string{
				"id", "checkId", "checkName", "type", "status", "severity", "message", "startedAt", "resolvedAt",
			})
			for _, inc := range incidents {
				resolvedAt := ""
				if inc.ResolvedAt != nil {
					resolvedAt = inc.ResolvedAt.UTC().Format(time.RFC3339)
				}
				_ = cw.Write([]string{
					inc.ID,
					inc.CheckID,
					inc.CheckName,
					inc.Type,
					inc.Status,
					inc.Severity,
					inc.Message,
					inc.StartedAt.UTC().Format(time.RFC3339),
					resolvedAt,
				})
			}
			cw.Flush()
			return
		}

		WriteAPIResponse(w, http.StatusOK, NewAPIResponse(incidents))
	}
}

// handleExportResults returns an http.HandlerFunc that exports check results as CSV or JSON.
func handleExportResults(store Store, retentionDays int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		format := ExportFormat(strings.TrimSpace(r.URL.Query().Get("format")))
		if format == "" {
			format = ExportJSON
		}

		checkID := strings.TrimSpace(r.URL.Query().Get("checkId"))
		days := retentionDays
		if d := r.URL.Query().Get("days"); d != "" {
			if parsed, err := strconv.Atoi(d); err == nil && parsed > 0 {
				days = parsed
			}
		}

		snap := store.Snapshot()
		cutoff := time.Now().UTC().AddDate(0, 0, -days)

		var results []CheckResult
		for _, res := range snap.Results {
			if !res.StartedAt.Before(cutoff) {
				if checkID == "" || res.CheckID == checkID {
					results = append(results, res)
				}
			}
		}

		sort.Slice(results, func(i, j int) bool {
			return results[i].StartedAt.Before(results[j].StartedAt)
		})

		if format == ExportCSV {
			w.Header().Set("Content-Type", "text/csv")
			w.Header().Set("Content-Disposition", "attachment; filename=results.csv")

			cw := csv.NewWriter(w)
			_ = cw.Write([]string{
				"id", "checkId", "name", "type", "server", "application",
				"status", "healthy", "durationMs", "startedAt", "finishedAt", "message",
			})
			for _, res := range results {
				_ = cw.Write([]string{
					res.ID,
					res.CheckID,
					res.Name,
					res.Type,
					res.Server,
					res.Application,
					res.Status,
					strconv.FormatBool(res.Healthy),
					strconv.FormatInt(res.DurationMs, 10),
					res.StartedAt.UTC().Format(time.RFC3339),
					res.FinishedAt.UTC().Format(time.RFC3339),
					res.Message,
				})
			}
			cw.Flush()
			return
		}

		WriteAPIResponse(w, http.StatusOK, NewAPIResponse(results))
	}
}
