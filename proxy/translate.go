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

// buildSimpleRequest creates a VictoriaLogs request with query, start, and end
// translated from Loki parameters. Used by endpoints that only need these three params.
func buildSimpleRequest(backend *url.URL, lokiParams url.Values, path string) (*http.Request, error) {
	q := url.Values{}
	q.Set("query", TranslateQuery(lokiParams.Get("query")))

	if start := lokiParams.Get("start"); start != "" {
		ts, err := TranslateTimestamp(start)
		if err != nil {
			return nil, fmt.Errorf("start: %w", err)
		}
		q.Set("start", ts)
	}

	if end := lokiParams.Get("end"); end != "" {
		ts, err := TranslateTimestamp(end)
		if err != nil {
			return nil, fmt.Errorf("end: %w", err)
		}
		q.Set("end", ts)
	}

	u := *backend
	u.Path = path
	u.RawQuery = q.Encode()

	return http.NewRequest(http.MethodGet, u.String(), nil)
}

// BuildFieldNamesRequest creates an HTTP request to the VictoriaLogs
// /select/logsql/field_names endpoint from Loki detected_labels/detected_fields parameters.
func BuildFieldNamesRequest(backend *url.URL, lokiParams url.Values) (*http.Request, error) {
	return buildSimpleRequest(backend, lokiParams, "/select/logsql/field_names")
}

// BuildStreamFieldNamesRequest creates an HTTP request to the VictoriaLogs
// /select/logsql/stream_field_names endpoint from Loki labels parameters.
func BuildStreamFieldNamesRequest(backend *url.URL, lokiParams url.Values) (*http.Request, error) {
	return buildSimpleRequest(backend, lokiParams, "/select/logsql/stream_field_names")
}

// BuildStreamFieldValuesRequest creates an HTTP request to the VictoriaLogs
// /select/logsql/stream_field_values endpoint from Loki label values parameters.
func BuildStreamFieldValuesRequest(backend *url.URL, lokiParams url.Values, fieldName string) (*http.Request, error) {
	req, err := buildSimpleRequest(backend, lokiParams, "/select/logsql/stream_field_values")
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("field_name", fieldName)
	req.URL.RawQuery = q.Encode()
	return req, nil
}

// BuildHitsRangeRequest creates an HTTP request to the VictoriaLogs
// /select/logsql/hits endpoint from Loki volume_range request parameters.
// Unlike BuildHitsRequest, the step is taken from the Loki request directly.
func BuildHitsRangeRequest(backend *url.URL, lokiParams url.Values) (*http.Request, error) {
	q := url.Values{}

	q.Set("query", TranslateQuery(lokiParams.Get("query")))

	if start := lokiParams.Get("start"); start != "" {
		ts, err := TranslateTimestamp(start)
		if err != nil {
			return nil, fmt.Errorf("start: %w", err)
		}
		q.Set("start", ts)
	}

	if end := lokiParams.Get("end"); end != "" {
		ts, err := TranslateTimestamp(end)
		if err != nil {
			return nil, fmt.Errorf("end: %w", err)
		}
		q.Set("end", ts)
	}

	if step := lokiParams.Get("step"); step != "" {
		q.Set("step", step)
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

	return http.NewRequest(http.MethodGet, u.String(), nil)
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

func BuildQueryRequest(backend *url.URL, lokiParams url.Values) (*http.Request, error) {
	q := url.Values{}
	q.Set("query", TranslateQuery(lokiParams.Get("query")))

	if t := lokiParams.Get("time"); t != "" {
		ts, err := TranslateTimestamp(t)
		if err != nil {
			return nil, fmt.Errorf("time: %w", err)
		}
		q.Set("start", ts)
		q.Set("end", ts)
	} else {
		if start := lokiParams.Get("start"); start != "" {
			ts, err := TranslateTimestamp(start)
			if err != nil {
				return nil, fmt.Errorf("start: %w", err)
			}
			q.Set("start", ts)
		}
		if end := lokiParams.Get("end"); end != "" {
			ts, err := TranslateTimestamp(end)
			if err != nil {
				return nil, fmt.Errorf("end: %w", err)
			}
			q.Set("end", ts)
		}
	}

	if limit := lokiParams.Get("limit"); limit != "" {
		q.Set("limit", limit)
	}

	u := *backend
	u.Path = "/select/logsql/query"
	u.RawQuery = q.Encode()

	return http.NewRequest(http.MethodGet, u.String(), nil)
}

func BuildStreamsRequest(backend *url.URL, lokiParams url.Values) (*http.Request, error) {
	return buildSimpleRequest(backend, lokiParams, "/select/logsql/streams")
}

func BuildFieldValuesRequest(backend *url.URL, lokiParams url.Values, fieldName string) (*http.Request, error) {
	req, err := buildSimpleRequest(backend, lokiParams, "/select/logsql/field_values")
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("field", fieldName)
	req.URL.RawQuery = q.Encode()
	return req, nil
}

func BuildStatsRequest(backend *url.URL, lokiParams url.Values) (*http.Request, error) {
	return buildSimpleRequest(backend, lokiParams, "/select/logsql/stats")
}
