package proxy

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TranslateQuery converts a Loki LogQL stream selector to a VictoriaLogs
// LogSQL query. Loki sends selectors like {job="foo"}, VictoriaLogs expects
// _stream:{job="foo"}.
func TranslateQuery(lokiQuery string) string {
	q := strings.TrimSpace(lokiQuery)
	if q == "" {
		return "*"
	}
	return "_stream:" + q
}

// parseTimestamp parses a timestamp string that may be either a nanosecond
// Unix epoch (e.g. "1700000000000000000") or an ISO 8601 / RFC3339 string
// (e.g. "2026-04-05T05:23:26.735Z"). Returns the parsed time in UTC.
func parseTimestamp(s string) (time.Time, error) {
	if ns, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(0, ns).UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("invalid timestamp %q: not a nanosecond epoch or RFC3339 value", s)
}

// TranslateTimestamp converts a timestamp string (nanosecond Unix epoch or
// RFC3339/ISO 8601) to an RFC3339 timestamp string for VictoriaLogs.
func TranslateTimestamp(raw string) (string, error) {
	t, err := parseTimestamp(raw)
	if err != nil {
		return "", err
	}
	return t.Format(time.RFC3339), nil
}

// BuildHitsRequest creates an HTTP request to the VictoriaLogs
// /select/logsql/hits endpoint from Loki volume request parameters.
func BuildHitsRequest(backend *url.URL, lokiParams url.Values) (*http.Request, error) {
	q := url.Values{}

	q.Set("query", TranslateQuery(lokiParams.Get("query")))

	startNs := lokiParams.Get("start")
	endNs := lokiParams.Get("end")

	if startNs != "" {
		ts, err := TranslateTimestamp(startNs)
		if err != nil {
			return nil, fmt.Errorf("start: %w", err)
		}
		q.Set("start", ts)
	}

	if endNs != "" {
		ts, err := TranslateTimestamp(endNs)
		if err != nil {
			return nil, fmt.Errorf("end: %w", err)
		}
		q.Set("end", ts)
	}

	// Calculate step as the entire time range (single bucket for total volume).
	if startNs != "" && endNs != "" {
		startTime, err1 := parseTimestamp(startNs)
		endTime, err2 := parseTimestamp(endNs)
		dur := time.Hour
		if err1 == nil && err2 == nil {
			dur = endTime.Sub(startTime)
			if dur <= 0 {
				dur = time.Hour
			}
		}
		q.Set("step", dur.String())
	} else {
		q.Set("step", "1h")
	}

	if limit := lokiParams.Get("limit"); limit != "" {
		q.Set("fields_limit", limit)
	}

	if targetLabels := lokiParams.Get("targetLabels"); targetLabels != "" {
		for _, label := range strings.Split(targetLabels, ",") {
			label = strings.TrimSpace(label)
			if label != "" {
				q.Add("field", label)
			}
		}
	}

	u := *backend
	u.Path = "/select/logsql/hits"
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	return req, nil
}
