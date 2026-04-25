package proxy

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// Implemented (translated to VictoriaLogs equivalents).
	volumePath              = "/loki/api/v1/index/volume"
	volumeRangePath         = "/loki/api/v1/index/volume_range"
	detectedLabelsPath      = "/loki/api/v1/detected_labels"
	detectedFieldsPath      = "/loki/api/v1/detected_fields"
	labelsPath              = "/loki/api/v1/labels"
	labelValuesPath         = "/loki/api/v1/label/{name}/values"
	patternsPath            = "/loki/api/v1/patterns"
	queryPath               = "/loki/api/v1/query"
	queryRangePath          = "/loki/api/v1/query_range"
	labelAliasPath          = "/loki/api/v1/label"
	seriesPath              = "/loki/api/v1/series"
	detectedFieldValuesPath = "/loki/api/v1/detected_field/{name}/values"
	indexStatsPath          = "/loki/api/v1/index/stats"
	pushPath                = "/loki/api/v1/push"
	promQueryPath           = "/api/prom/query"
	promLabelPath           = "/api/prom/label"
	promLabelValuesPath     = "/api/prom/label/{name}/values"
	promSeriesPath          = "/api/prom/series"
	promPushPath            = "/api/prom/push"
	otlpLogsPath            = "/otlp/v1/logs"
	readyPath               = "/ready"
	buildinfoPath           = "/loki/api/v1/status/buildinfo"
	tailPath                = "/loki/api/v1/tail"
	promTailPath            = "/api/prom/tail"
	drilldownLimitsPath     = "/loki/api/v1/drilldown-limits"
)

// notImplementedPaths lists every Loki API path that the compat layer does not
// yet translate. Each is registered as 501 Not Implemented so callers get an
// explicit error rather than a confusing response from VictoriaLogs.
var notImplementedPaths = []string{
	// Index
	"/loki/api/v1/index/shards",
	// Ruler / alerting
	"/loki/api/v1/rules",
	"/loki/api/v1/rules/{namespace}",
	"/loki/api/v1/rules/{namespace}/{groupName}",
	"/prometheus/api/v1/rules",
	"/prometheus/api/v1/alerts",
	"/api/prom/rules",
	"/api/prom/rules/{namespace}",
	"/api/prom/rules/{namespace}/{groupName}",
	// Log deletion
	"/loki/api/v1/delete",
	"/loki/api/v1/cache/generation_numbers",
}

// NewProxy returns an http.Handler that intercepts Loki volume requests and
// translates them to VictoriaLogs /select/logsql/hits, while passing all other
// requests through to the backend unchanged.
func NewProxy(backend *url.URL) http.Handler {
	rp := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(backend)
			r.Out.Host = backend.Host
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc(volumePath, func(w http.ResponseWriter, r *http.Request) {
		handleVolume(w, r, backend)
	})
	mux.HandleFunc(volumeRangePath, func(w http.ResponseWriter, r *http.Request) {
		handleVolumeRange(w, r, backend)
	})
	mux.HandleFunc(detectedLabelsPath, func(w http.ResponseWriter, r *http.Request) {
		handleDetectedLabels(w, r, backend)
	})
	mux.HandleFunc(detectedFieldsPath, func(w http.ResponseWriter, r *http.Request) {
		handleDetectedFields(w, r, backend)
	})
	mux.HandleFunc(labelsPath, func(w http.ResponseWriter, r *http.Request) {
		handleLabels(w, r, backend)
	})
	mux.HandleFunc(labelValuesPath, func(w http.ResponseWriter, r *http.Request) {
		handleLabelValues(w, r, backend)
	})
	mux.HandleFunc(patternsPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"success","data":[]}`))
	})
	mux.HandleFunc(drilldownLimitsPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Shape mirrors grafana/logs-drilldown's LokiConfig type. All numeric
		// and duration limits use 0 / "0s" — verified against
		// timePicker.ts:filterInvalidTimeOptions which falsy-checks the value
		// before applying the gate, so 0 means "unlimited" rather than "zero".
		// pattern_ingester_enabled is false because VL has no pattern ingester
		// (matches our /loki/api/v1/patterns stub returning an empty list).
		w.Write([]byte(`{` +
			`"version":"unknown",` +
			`"pattern_ingester_enabled":false,` +
			`"limits":{` +
			`"retention_period":"0s",` +
			`"max_query_series":0,` +
			`"max_query_length":"0s",` +
			`"max_query_lookback":"0s",` +
			`"max_query_range":"0s",` +
			`"max_query_bytes_read":0,` +
			`"max_entries_limit_per_query":0,` +
			`"query_timeout":"0s",` +
			`"volume_enabled":true,` +
			`"volume_max_series":0,` +
			`"metric_aggregation_enabled":false,` +
			`"pattern_persistence_enabled":false,` +
			`"discover_log_levels":true` +
			`}}`))
	})
	mux.HandleFunc(queryPath, func(w http.ResponseWriter, r *http.Request) {
		handleQuery(w, r, backend)
	})
	mux.HandleFunc(queryRangePath, func(w http.ResponseWriter, r *http.Request) {
		handleQuery(w, r, backend)
	})
	mux.HandleFunc(labelAliasPath, func(w http.ResponseWriter, r *http.Request) {
		handleLabels(w, r, backend)
	})
	mux.HandleFunc(seriesPath, func(w http.ResponseWriter, r *http.Request) {
		handleSeries(w, r, backend)
	})
	mux.HandleFunc(detectedFieldValuesPath, func(w http.ResponseWriter, r *http.Request) {
		handleDetectedFieldValues(w, r, backend)
	})
	mux.HandleFunc(indexStatsPath, func(w http.ResponseWriter, r *http.Request) {
		handleIndexStats(w, r, backend)
	})
	mux.HandleFunc(pushPath, func(w http.ResponseWriter, r *http.Request) {
		handleProxyRewrite(w, r, backend, "/insert/loki/api/v1/push")
	})
	mux.HandleFunc(promPushPath, func(w http.ResponseWriter, r *http.Request) {
		handleProxyRewrite(w, r, backend, "/insert/loki/api/v1/push")
	})
	mux.HandleFunc(otlpLogsPath, func(w http.ResponseWriter, r *http.Request) {
		handleProxyRewrite(w, r, backend, "/insert/opentelemetry/api/logs/export")
	})
	mux.HandleFunc(promQueryPath, func(w http.ResponseWriter, r *http.Request) {
		handleQuery(w, r, backend)
	})
	mux.HandleFunc(promLabelPath, func(w http.ResponseWriter, r *http.Request) {
		handleLabels(w, r, backend)
	})
	mux.HandleFunc(promLabelValuesPath, func(w http.ResponseWriter, r *http.Request) {
		handleLabelValues(w, r, backend)
	})
	mux.HandleFunc(promSeriesPath, func(w http.ResponseWriter, r *http.Request) {
		handleSeries(w, r, backend)
	})
	mux.HandleFunc(readyPath, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc(buildinfoPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"version":"2.9.0","revision":"","branch":"","buildUser":"","buildDate":"","goVersion":""}`))
	})
	mux.HandleFunc(tailPath, func(w http.ResponseWriter, r *http.Request) {
		handleTail(w, r, backend)
	})
	mux.HandleFunc(promTailPath, func(w http.ResponseWriter, r *http.Request) {
		handleTail(w, r, backend)
	})
	for _, path := range notImplementedPaths {
		mux.HandleFunc(path, notImplemented)
	}
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("unhandled request: %s %s", r.Method, r.URL.Path)
		rp.ServeHTTP(w, r)
	}))
	return mux
}

// copyAuthHeaders forwards authentication-related headers from the incoming
// client request to an outgoing backend request so credentials aren't stripped.
func copyAuthHeaders(dst, src *http.Request) {
	for _, h := range []string{"Authorization", "X-Scope-OrgID"} {
		if v := src.Header.Get(h); v != "" {
			dst.Header.Set(h, v)
		}
	}
}

func notImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func handleVolume(w http.ResponseWriter, r *http.Request, backend *url.URL) {
	params, err := extractParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hitsReq, err := BuildHitsRequest(backend, params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	copyAuthHeaders(hitsReq, r)

	resp, err := http.DefaultClient.Do(hitsReq)
	if err != nil {
		log.Printf("backend request failed: %v", err)
		http.Error(w, "backend request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read backend response", http.StatusBadGateway)
		return
	}

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}

	result, err := ConvertHitsToVolume(body, time.Now())
	if err != nil {
		log.Printf("response conversion failed: %v", err)
		http.Error(w, "response conversion failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(result)
}

// handleTranslated is a generic handler for endpoints that follow the standard
// pattern: extract params → build VL request → execute → convert response.
func handleTranslated(
	w http.ResponseWriter,
	r *http.Request,
	backend *url.URL,
	buildReq func(*url.URL, url.Values) (*http.Request, error),
	convert func([]byte) ([]byte, error),
) {
	params, err := extractParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req, err := buildReq(backend, params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	copyAuthHeaders(req, r)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("backend request failed: %v", err)
		http.Error(w, "backend request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read backend response", http.StatusBadGateway)
		return
	}

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}

	result, err := convert(body)
	if err != nil {
		log.Printf("response conversion failed: %v", err)
		http.Error(w, "response conversion failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(result)
}

func handleVolumeRange(w http.ResponseWriter, r *http.Request, backend *url.URL) {
	handleTranslated(w, r, backend, BuildHitsRangeRequest, ConvertHitsToVolumeRange)
}

func handleDetectedLabels(w http.ResponseWriter, r *http.Request, backend *url.URL) {
	handleTranslated(w, r, backend, BuildFieldNamesRequest, ConvertFieldNamesToDetectedLabels)
}

func handleDetectedFields(w http.ResponseWriter, r *http.Request, backend *url.URL) {
	handleTranslated(w, r, backend, BuildFieldNamesRequest, ConvertFieldNamesToDetectedFields)
}

func handleLabels(w http.ResponseWriter, r *http.Request, backend *url.URL) {
	handleTranslated(w, r, backend, BuildStreamFieldNamesRequest, ConvertFieldNamesToLabels)
}

func handleLabelValues(w http.ResponseWriter, r *http.Request, backend *url.URL) {
	fieldName := r.PathValue("name")
	params, err := extractParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req, err := BuildStreamFieldValuesRequest(backend, params, fieldName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	copyAuthHeaders(req, r)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("backend request failed: %v", err)
		http.Error(w, "backend request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read backend response", http.StatusBadGateway)
		return
	}

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}

	result, err := ConvertFieldNamesToLabels(body)
	if err != nil {
		log.Printf("response conversion failed: %v", err)
		http.Error(w, "response conversion failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(result)
}

// isProbeQuery detects synthetic Prometheus liveness-probe queries (e.g.
// "vector(1)+vector(1)") that Grafana's Loki datasource health check sends.
// A real LogQL query always contains a stream selector "{...}"; queries with
// no '{' aren't LogQL — they're constant Prometheus expressions evaluated
// locally so they don't hit VL.
func isProbeQuery(q string) bool {
	s := strings.TrimSpace(q)
	return s != "" && !strings.Contains(s, "{")
}

func handleQuery(w http.ResponseWriter, r *http.Request, backend *url.URL) {
	params, err := extractParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if isProbeQuery(params.Get("query")) {
		writeProbeResponse(w, r, params)
		return
	}
	handleTranslated(w, r, backend, BuildQueryRequest, ConvertQueryToStreams)
}

// writeProbeResponse evaluates a constant Prometheus probe query and writes a
// Prometheus-shaped response — vector for /query, matrix for /query_range.
// If the expression isn't in the subset we can evaluate, return 501 so the
// gap is visible rather than masked by a fake empty result.
func writeProbeResponse(w http.ResponseWriter, r *http.Request, params url.Values) {
	q := params.Get("query")
	v, ok := evalProbe(q)
	if !ok {
		http.Error(w, "probe query not implemented: "+q, http.StatusNotImplemented)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	valStr := strconv.FormatFloat(v, 'f', -1, 64)

	if strings.HasSuffix(r.URL.Path, "/query_range") {
		samples := probeRangeSamples(params)
		results := make([]lokiVolumeRangeResult, 1)
		results[0].Metric = map[string]string{}
		results[0].Values = make([][2]interface{}, len(samples))
		for i, ts := range samples {
			results[0].Values[i] = [2]interface{}{float64(ts), valStr}
		}
		body, _ := json.Marshal(lokiVolumeRangeResponse{
			Status: "success",
			Data:   lokiVolumeRangeData{ResultType: "matrix", Result: results},
		})
		w.Write(body)
		return
	}

	ts := probeInstantSample(params)
	body, _ := json.Marshal(lokiVolumeResponse{
		Status: "success",
		Data: lokiVolumeData{
			ResultType: "vector",
			Result: []lokiVolumeResult{{
				Metric: map[string]string{},
				Value:  [2]interface{}{float64(ts), valStr},
			}},
		},
	})
	w.Write(body)
}

// probeInstantSample picks the timestamp for an instant-query probe response:
// the request's `time` param, falling back to `start`, falling back to now.
func probeInstantSample(params url.Values) int64 {
	for _, key := range []string{"time", "start"} {
		if raw := params.Get(key); raw != "" {
			if t, err := parseTimestamp(raw); err == nil {
				return t.Unix()
			}
		}
	}
	return time.Now().Unix()
}

// probeRangeSamples produces the timestamps to sample at for a range probe.
// Walks from start to end in `step` increments (default 15s, capped at 11
// points to keep the response tiny), or falls back to a single now-sample.
func probeRangeSamples(params url.Values) []int64 {
	start, errS := parseTimestamp(params.Get("start"))
	end, errE := parseTimestamp(params.Get("end"))
	if errS != nil || errE != nil || !end.After(start) {
		return []int64{time.Now().Unix()}
	}

	step := 15 * time.Second
	if raw := params.Get("step"); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			step = d
		} else if secs, err := strconv.ParseFloat(raw, 64); err == nil && secs > 0 {
			step = time.Duration(secs * float64(time.Second))
		}
	}

	const maxPoints = 11
	span := end.Sub(start)
	if step < span/maxPoints {
		step = span / maxPoints
	}

	out := make([]int64, 0, maxPoints+1)
	for t := start; !t.After(end) && len(out) <= maxPoints; t = t.Add(step) {
		out = append(out, t.Unix())
	}
	return out
}

func handleSeries(w http.ResponseWriter, r *http.Request, backend *url.URL) {
	handleTranslated(w, r, backend, BuildStreamsRequest, ConvertStreamsToSeries)
}

func handleDetectedFieldValues(w http.ResponseWriter, r *http.Request, backend *url.URL) {
	fieldName := r.PathValue("name")
	params, err := extractParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	req, err := BuildFieldValuesRequest(backend, params, fieldName)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	copyAuthHeaders(req, r)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("backend request failed: %v", err)
		http.Error(w, "backend request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read backend response", http.StatusBadGateway)
		return
	}

	if resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		w.Write(body)
		return
	}

	result, err := ConvertFieldValuesToDetectedFieldValues(body)
	if err != nil {
		log.Printf("response conversion failed: %v", err)
		http.Error(w, "response conversion failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(result)
}

func handleIndexStats(w http.ResponseWriter, r *http.Request, backend *url.URL) {
	handleTranslated(w, r, backend, BuildStatsRequest, ConvertStatsToIndexStats)
}

// handleProxyRewrite forwards the request to the backend with a different path,
// proxying headers and body unchanged. Used for push/ingest endpoints where
// VictoriaLogs uses a different URL prefix than Loki.
func handleProxyRewrite(w http.ResponseWriter, r *http.Request, backend *url.URL, targetPath string) {
	u := *backend
	u.Path = targetPath
	u.RawQuery = r.URL.RawQuery

	proxyReq, err := http.NewRequest(r.Method, u.String(), r.Body)
	if err != nil {
		http.Error(w, "failed to create request", http.StatusInternalServerError)
		return
	}
	for key, vals := range r.Header {
		for _, v := range vals {
			proxyReq.Header.Add(key, v)
		}
	}

	resp, err := http.DefaultClient.Do(proxyReq)
	if err != nil {
		log.Printf("backend request failed: %v", err)
		http.Error(w, "backend request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for key, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(key, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// extractParams gets query parameters from GET query string or POST form body.
func extractParams(r *http.Request) (url.Values, error) {
	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			return nil, err
		}
		return r.Form, nil
	}
	return r.URL.Query(), nil
}
