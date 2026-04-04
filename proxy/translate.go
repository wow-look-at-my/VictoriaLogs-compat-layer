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

// TranslateTimestamp converts a nanosecond Unix epoch string (as Loki sends)
// to an RFC3339 timestamp string for VictoriaLogs.
func TranslateTimestamp(nanos string) (string, error) {
	ns, err := strconv.ParseInt(nanos, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid timestamp %q: %w", nanos, err)
	}
	t := time.Unix(0, ns).UTC()
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
		startInt, _ := strconv.ParseInt(startNs, 10, 64)
		endInt, _ := strconv.ParseInt(endNs, 10, 64)
		dur := time.Duration(endInt - startInt)
		if dur <= 0 {
			dur = time.Hour
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
