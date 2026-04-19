package monitoring

import (
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Response types
// ---------------------------------------------------------------------------

// UptimeStats for a check over a period
type UptimeStats struct {
	CheckID       string  `json:"checkId"`
	CheckName     string  `json:"checkName"`
	Period        string  `json:"period"`
	TotalResults  int     `json:"totalResults"`
	HealthyCount  int     `json:"healthyCount"`
	UptimePct     float64 `json:"uptimePct"`
	AvgDurationMs float64 `json:"avgDurationMs"`
	MaxDurationMs int64   `json:"maxDurationMs"`
	MinDurationMs int64   `json:"minDurationMs"`
}

// ResponseTimeBucket for time-series charting
type ResponseTimeBucket struct {
	Timestamp     time.Time `json:"timestamp"`
	AvgDurationMs float64   `json:"avgDurationMs"`
	P50DurationMs float64   `json:"p50DurationMs"`
	P95DurationMs float64   `json:"p95DurationMs"`
	P99DurationMs float64   `json:"p99DurationMs"`
	MaxDurationMs int64     `json:"maxDurationMs"`
	MinDurationMs int64     `json:"minDurationMs"`
	Count         int       `json:"count"`
}

// StatusTimelineEntry for timeline visualization
type StatusTimelineEntry struct {
	Timestamp  time.Time `json:"timestamp"`
	Status     string    `json:"status"`
	DurationMs int64     `json:"durationMs"`
	Message    string    `json:"message,omitempty"`
}

// FailureRateEntry for failure rate analytics
type FailureRateEntry struct {
	Group        string  `json:"group"`
	TotalResults int     `json:"totalResults"`
	FailedCount  int     `json:"failedCount"`
	FailureRate  float64 `json:"failureRate"`
}

// IncidentStats for incident analytics
type IncidentStats struct {
	Total        int            `json:"total"`
	Open         int            `json:"open"`
	Acknowledged int            `json:"acknowledged"`
	Resolved     int            `json:"resolved"`
	MTTRAMinutes float64        `json:"mttaMinutes"`
	MTTRMinutes  float64        `json:"mttrMinutes"`
	BySeverity   map[string]int `json:"bySeverity"`
}

// OverviewStats for dashboard hero cards
type OverviewStats struct {
	TotalChecks     int            `json:"totalChecks"`
	EnabledChecks   int            `json:"enabledChecks"`
	HealthyChecks   int            `json:"healthyChecks"`
	ActiveIncidents int            `json:"activeIncidents"`
	AvgUptimePct    float64        `json:"avgUptimePct"`
	ChecksByType    map[string]int `json:"checksByType"`
	ChecksByServer  map[string]int `json:"checksByServer"`
}

// CheckDetail is a rich view of a single check for the frontend
type CheckDetail struct {
	Config        CheckConfig   `json:"config"`
	LatestResult  *CheckResult  `json:"latestResult,omitempty"`
	Uptime24h     float64       `json:"uptime24h"`
	Uptime7d      float64       `json:"uptime7d"`
	AvgDurationMs float64       `json:"avgDurationMs"`
	RecentResults []CheckResult `json:"recentResults"`
	OpenIncidents []Incident    `json:"openIncidents,omitempty"`
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// parsePeriod converts a period string to a time.Duration.
func parsePeriod(period string) time.Duration {
	switch period {
	case "7d":
		return 7 * 24 * time.Hour
	case "30d":
		return 30 * 24 * time.Hour
	case "90d":
		return 90 * 24 * time.Hour
	default: // "24h" or anything unrecognised
		return 24 * time.Hour
	}
}

// parsePeriodLabel returns the canonical label for a period string.
func parsePeriodLabel(period string) string {
	switch period {
	case "7d", "30d", "90d":
		return period
	default:
		return "24h"
	}
}

// parseInterval converts an interval string to a time.Duration.
func parseInterval(interval string) time.Duration {
	switch interval {
	case "6h":
		return 6 * time.Hour
	case "1d":
		return 24 * time.Hour
	default: // "1h"
		return time.Hour
	}
}

// computeUptime computes UptimeStats for a single check over the given period.
func computeUptime(results []CheckResult, checkID string, period time.Duration) UptimeStats {
	cutoff := time.Now().UTC().Add(-period)

	var (
		total   int
		healthy int
		sumMs   int64
		maxMs   int64
		minMs   int64 = math.MaxInt64
		name    string
	)

	for i := range results {
		r := &results[i]
		if r.CheckID != checkID || r.StartedAt.Before(cutoff) {
			continue
		}
		if name == "" {
			name = r.Name
		}
		total++
		if r.Healthy {
			healthy++
		}
		sumMs += r.DurationMs
		if r.DurationMs > maxMs {
			maxMs = r.DurationMs
		}
		if r.DurationMs < minMs {
			minMs = r.DurationMs
		}
	}

	if total == 0 {
		minMs = 0
	}

	var uptimePct float64
	var avgMs float64
	if total > 0 {
		uptimePct = math.Round(float64(healthy)/float64(total)*10000) / 100
		avgMs = math.Round(float64(sumMs)/float64(total)*100) / 100
	}

	return UptimeStats{
		CheckID:       checkID,
		CheckName:     name,
		TotalResults:  total,
		HealthyCount:  healthy,
		UptimePct:     uptimePct,
		AvgDurationMs: avgMs,
		MaxDurationMs: maxMs,
		MinDurationMs: minMs,
	}
}

// computeResponseTimeBuckets aggregates results into time buckets.
func computeResponseTimeBuckets(results []CheckResult, checkID string, period, interval time.Duration) []ResponseTimeBucket {
	now := time.Now().UTC()
	cutoff := now.Add(-period)

	// Gather matching results
	type entry struct {
		ts time.Time
		ms int64
	}
	var entries []entry
	for i := range results {
		r := &results[i]
		if r.CheckID != checkID || r.StartedAt.Before(cutoff) {
			continue
		}
		entries = append(entries, entry{ts: r.StartedAt, ms: r.DurationMs})
	}

	if len(entries) == 0 {
		return []ResponseTimeBucket{}
	}

	// Build time slots
	start := cutoff.Truncate(interval)
	var buckets []ResponseTimeBucket

	for t := start; t.Before(now); t = t.Add(interval) {
		bucketEnd := t.Add(interval)
		var durations []int64

		for _, e := range entries {
			if !e.ts.Before(t) && e.ts.Before(bucketEnd) {
				durations = append(durations, e.ms)
			}
		}

		if len(durations) == 0 {
			continue
		}

		sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })

		var sum int64
		maxMs := durations[0]
		minMs := durations[0]
		for _, d := range durations {
			sum += d
			if d > maxMs {
				maxMs = d
			}
			if d < minMs {
				minMs = d
			}
		}

		buckets = append(buckets, ResponseTimeBucket{
			Timestamp:     t,
			AvgDurationMs: math.Round(float64(sum)/float64(len(durations))*100) / 100,
			P50DurationMs: percentile(durations, 0.50),
			P95DurationMs: percentile(durations, 0.95),
			P99DurationMs: percentile(durations, 0.99),
			MaxDurationMs: maxMs,
			MinDurationMs: minMs,
			Count:         len(durations),
		})
	}

	return buckets
}

// percentile returns the pct-th percentile from a pre-sorted slice.
func percentile(sorted []int64, pct float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := pct * float64(len(sorted)-1)
	lower := int(math.Floor(idx))
	upper := int(math.Ceil(idx))
	if lower == upper || upper >= len(sorted) {
		return float64(sorted[lower])
	}
	frac := idx - float64(lower)
	return math.Round((float64(sorted[lower])*(1-frac)+float64(sorted[upper])*frac)*100) / 100
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// handleAnalyticsUptime returns uptime stats for one or all checks.
// GET /api/v1/analytics/uptime?checkId=&period=24h|7d|30d|90d
func (s *Service) handleAnalyticsUptime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, http.ErrNotSupported)
		return
	}

	checkID := strings.TrimSpace(r.URL.Query().Get("checkId"))
	period := strings.TrimSpace(r.URL.Query().Get("period"))
	dur := parsePeriod(period)
	label := parsePeriodLabel(period)

	snap := s.store.Snapshot()

	if checkID != "" {
		stats := computeUptime(snap.Results, checkID, dur)
		stats.Period = label
		writeAPIResponse(w, http.StatusOK, NewAPIResponse(stats))
		return
	}

	var all []UptimeStats
	for i := range snap.Checks {
		if !snap.Checks[i].IsEnabled() {
			continue
		}
		stats := computeUptime(snap.Results, snap.Checks[i].ID, dur)
		stats.Period = label
		all = append(all, stats)
	}
	if all == nil {
		all = []UptimeStats{}
	}
	writeAPIResponse(w, http.StatusOK, NewAPIResponse(all))
}

// handleAnalyticsResponseTimes returns response-time buckets for charting.
// GET /api/v1/analytics/response-times?checkId=&period=24h|7d&interval=1h|6h|1d
func (s *Service) handleAnalyticsResponseTimes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, http.ErrNotSupported)
		return
	}

	checkID := strings.TrimSpace(r.URL.Query().Get("checkId"))
	if checkID == "" {
		writeAPIError(w, http.StatusBadRequest, errMissingID)
		return
	}

	period := strings.TrimSpace(r.URL.Query().Get("period"))
	interval := strings.TrimSpace(r.URL.Query().Get("interval"))

	dur := parsePeriod(period)
	intv := parseInterval(interval)

	snap := s.store.Snapshot()
	buckets := computeResponseTimeBuckets(snap.Results, checkID, dur, intv)
	writeAPIResponse(w, http.StatusOK, NewAPIResponse(buckets))
}

// handleAnalyticsStatusTimeline returns chronological status entries for a check.
// GET /api/v1/analytics/status-timeline?checkId=&days=7
func (s *Service) handleAnalyticsStatusTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, http.ErrNotSupported)
		return
	}

	checkID := strings.TrimSpace(r.URL.Query().Get("checkId"))
	if checkID == "" {
		writeAPIError(w, http.StatusBadRequest, errMissingID)
		return
	}

	days := queryInt(r, "days", 7)
	cutoff := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)

	snap := s.store.Snapshot()

	var timeline []StatusTimelineEntry
	for i := range snap.Results {
		res := &snap.Results[i]
		if res.CheckID != checkID || res.StartedAt.Before(cutoff) {
			continue
		}
		timeline = append(timeline, StatusTimelineEntry{
			Timestamp:  res.StartedAt,
			Status:     res.Status,
			DurationMs: res.DurationMs,
			Message:    res.Message,
		})
	}

	// Sort chronologically
	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i].Timestamp.Before(timeline[j].Timestamp)
	})

	if timeline == nil {
		timeline = []StatusTimelineEntry{}
	}
	writeAPIResponse(w, http.StatusOK, NewAPIResponse(timeline))
}

// handleAnalyticsFailureRate returns failure rates grouped by server, application, or type.
// GET /api/v1/analytics/failure-rate?period=24h|7d|30d&groupBy=server|application|type
func (s *Service) handleAnalyticsFailureRate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, http.ErrNotSupported)
		return
	}

	period := strings.TrimSpace(r.URL.Query().Get("period"))
	groupBy := strings.TrimSpace(r.URL.Query().Get("groupBy"))
	dur := parsePeriod(period)
	cutoff := time.Now().UTC().Add(-dur)

	snap := s.store.Snapshot()

	type counts struct {
		total  int
		failed int
	}
	groups := make(map[string]*counts)

	for i := range snap.Results {
		res := &snap.Results[i]
		if res.StartedAt.Before(cutoff) {
			continue
		}

		var key string
		switch groupBy {
		case "application":
			key = res.Application
		case "type":
			key = res.Type
		default: // "server"
			key = res.Server
		}
		if key == "" {
			key = "(none)"
		}

		c, ok := groups[key]
		if !ok {
			c = &counts{}
			groups[key] = c
		}
		c.total++
		if !res.Healthy {
			c.failed++
		}
	}

	entries := make([]FailureRateEntry, 0, len(groups))
	for g, c := range groups {
		var rate float64
		if c.total > 0 {
			rate = math.Round(float64(c.failed)/float64(c.total)*10000) / 100
		}
		entries = append(entries, FailureRateEntry{
			Group:        g,
			TotalResults: c.total,
			FailedCount:  c.failed,
			FailureRate:  rate,
		})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].FailureRate > entries[j].FailureRate })

	writeAPIResponse(w, http.StatusOK, NewAPIResponse(entries))
}

// handleAnalyticsMTTR returns incident statistics including MTTA and MTTR.
// GET /api/v1/analytics/incidents
func (s *Service) handleAnalyticsMTTR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, http.ErrNotSupported)
		return
	}

	stats := IncidentStats{
		BySeverity: make(map[string]int),
	}

	if s.incidentManager == nil || s.incidentManager.repo == nil {
		writeAPIResponse(w, http.StatusOK, NewAPIResponse(stats))
		return
	}

	incidents, err := s.incidentManager.repo.ListIncidents()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}

	var totalTTAMin, totalTTRMin float64
	var ackCount, resolveCount int

	for i := range incidents {
		inc := &incidents[i]
		stats.Total++
		stats.BySeverity[inc.Severity]++

		switch inc.Status {
		case "open":
			stats.Open++
		case "acknowledged":
			stats.Acknowledged++
		case "resolved":
			stats.Resolved++
		}

		if inc.AcknowledgedAt != nil {
			tta := inc.AcknowledgedAt.Sub(inc.StartedAt).Minutes()
			totalTTAMin += tta
			ackCount++
		}
		if inc.ResolvedAt != nil {
			ttr := inc.ResolvedAt.Sub(inc.StartedAt).Minutes()
			totalTTRMin += ttr
			resolveCount++
		}
	}

	if ackCount > 0 {
		stats.MTTRAMinutes = math.Round(totalTTAMin/float64(ackCount)*100) / 100
	}
	if resolveCount > 0 {
		stats.MTTRMinutes = math.Round(totalTTRMin/float64(resolveCount)*100) / 100
	}

	writeAPIResponse(w, http.StatusOK, NewAPIResponse(stats))
}

// handleStatsOverview returns a single payload for dashboard hero cards.
// GET /api/v1/stats/overview
func (s *Service) handleStatsOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, http.ErrNotSupported)
		return
	}

	snap := s.store.Snapshot()

	overview := OverviewStats{
		ChecksByType:   make(map[string]int),
		ChecksByServer: make(map[string]int),
	}

	overview.TotalChecks = len(snap.Checks)

	enabledIDs := make(map[string]bool)
	for i := range snap.Checks {
		c := &snap.Checks[i]
		if c.IsEnabled() {
			overview.EnabledChecks++
			enabledIDs[c.ID] = true
		}
		overview.ChecksByType[c.Type]++
		if c.Server != "" {
			overview.ChecksByServer[c.Server]++
		}
	}

	// Determine healthy checks from latest results per check
	latestByCheck := latestResultPerCheck(snap.Results)
	for id, res := range latestByCheck {
		if enabledIDs[id] && res.Healthy {
			overview.HealthyChecks++
		}
	}

	// Active incidents
	if s.incidentManager != nil && s.incidentManager.repo != nil {
		incidents, err := s.incidentManager.repo.ListIncidents()
		if err == nil {
			for i := range incidents {
				if incidents[i].Status == "open" || incidents[i].Status == "acknowledged" {
					overview.ActiveIncidents++
				}
			}
		}
	}

	// Average uptime across enabled checks (24h window)
	dur24h := 24 * time.Hour
	var uptimeSum float64
	var uptimeCount int
	for id := range enabledIDs {
		stats := computeUptime(snap.Results, id, dur24h)
		if stats.TotalResults > 0 {
			uptimeSum += stats.UptimePct
			uptimeCount++
		}
	}
	if uptimeCount > 0 {
		overview.AvgUptimePct = math.Round(uptimeSum/float64(uptimeCount)*100) / 100
	}

	writeAPIResponse(w, http.StatusOK, NewAPIResponse(overview))
}

// handleGetCheck returns a rich CheckDetail for a single check.
// Designed for GET /api/v1/checks/{id}
func (s *Service) handleGetCheck(w http.ResponseWriter, r *http.Request, checkID string) {
	if r.Method != http.MethodGet {
		writeAPIError(w, http.StatusMethodNotAllowed, http.ErrNotSupported)
		return
	}

	snap := s.store.Snapshot()

	// Find the check config
	var found *CheckConfig
	for i := range snap.Checks {
		if snap.Checks[i].ID == checkID {
			found = &snap.Checks[i]
			break
		}
	}
	if found == nil {
		writeAPIError(w, http.StatusNotFound, errCheckNotFound)
		return
	}

	// Gather results for this check, newest first
	var checkResults []CheckResult
	for i := range snap.Results {
		if snap.Results[i].CheckID == checkID {
			checkResults = append(checkResults, snap.Results[i])
		}
	}
	sort.Slice(checkResults, func(i, j int) bool {
		return checkResults[i].StartedAt.After(checkResults[j].StartedAt)
	})

	detail := CheckDetail{
		Config: *found,
	}

	if len(checkResults) > 0 {
		detail.LatestResult = &checkResults[0]
	}

	// Uptime calculations
	up24 := computeUptime(snap.Results, checkID, 24*time.Hour)
	up7d := computeUptime(snap.Results, checkID, 7*24*time.Hour)
	detail.Uptime24h = up24.UptimePct
	detail.Uptime7d = up7d.UptimePct

	// Avg duration from 24h window
	detail.AvgDurationMs = up24.AvgDurationMs

	// Recent results — cap at 50
	limit := 50
	if len(checkResults) < limit {
		limit = len(checkResults)
	}
	detail.RecentResults = checkResults[:limit]

	// Open incidents
	if s.incidentManager != nil && s.incidentManager.repo != nil {
		inc, err := s.incidentManager.repo.FindOpenIncident(checkID)
		if err == nil && inc.ID != "" {
			detail.OpenIncidents = []Incident{inc}
		}
	}

	writeAPIResponse(w, http.StatusOK, NewAPIResponse(detail))
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// latestResultPerCheck returns the most recent CheckResult for each checkID.
func latestResultPerCheck(results []CheckResult) map[string]CheckResult {
	latest := make(map[string]CheckResult)
	for i := range results {
		r := &results[i]
		if prev, ok := latest[r.CheckID]; !ok || r.StartedAt.After(prev.StartedAt) {
			latest[r.CheckID] = *r
		}
	}
	return latest
}

// errMissingID is returned when a required checkId parameter is absent.
var errMissingID = http.ErrAbortHandler // replaced below

func init() {
	errMissingID = errMissingCheckID
}

// errMissingCheckID is a concrete error value.
var errMissingCheckID = &apiErr{"checkId query parameter is required"}

// errCheckNotFound is returned when a check is not found.
var errCheckNotFound = &apiErr{"check not found"}

type apiErr struct{ msg string }

func (e *apiErr) Error() string { return e.msg }
