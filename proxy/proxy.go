package proxy

import (
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

const (
	// Implemented (translated to VictoriaLogs equivalents).
	volumePath         = "/loki/api/v1/index/volume"
	volumeRangePath    = "/loki/api/v1/index/volume_range"
	detectedLabelsPath = "/loki/api/v1/detected_labels"
	detectedFieldsPath = "/loki/api/v1/detected_fields"
	labelsPath         = "/loki/api/v1/labels"
	labelValuesPath    = "/loki/api/v1/label/{name}/values"
	patternsPath       = "/loki/api/v1/patterns"
)

// notImplementedPaths lists every Loki API path that the compat layer does not
// yet translate. Each is registered as 501 Not Implemented so callers get an
// explicit error rather than a confusing response from VictoriaLogs.
var notImplementedPaths = []string{
	// Query
	"/loki/api/v1/query",
	"/loki/api/v1/query_range",
	// Push / ingest
	"/loki/api/v1/push",
	"/api/prom/push",
	"/otlp/v1/logs",
	// Label metadata
	"/loki/api/v1/label",
	"/loki/api/v1/series",
	// Detected field values
	"/loki/api/v1/detected_field/{name}/values",
	// Index
	"/loki/api/v1/index/stats",
	"/loki/api/v1/index/shards",
	// Live tail
	"/loki/api/v1/tail",
	"/api/prom/tail",
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
	// Legacy prometheus-compat query API
	"/api/prom/query",
	"/api/prom/label",
	"/api/prom/label/{name}/values",
	"/api/prom/series",
	// Management
	"/ready",
	"/loki/api/v1/status/buildinfo",
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
	for _, path := range notImplementedPaths {
		mux.HandleFunc(path, notImplemented)
	}
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("unhandled request: %s %s", r.Method, r.URL.Path)
		rp.ServeHTTP(w, r)
	}))
	return mux
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
