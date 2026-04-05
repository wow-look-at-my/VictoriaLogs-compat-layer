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
	volumePath         = "/loki/api/v1/index/volume"
	volumeRangePath    = "/loki/api/v1/index/volume_range"
	detectedLabelsPath = "/loki/api/v1/detected_labels"
	detectedFieldsPath = "/loki/api/v1/detected_fields"
	labelsPath         = "/loki/api/v1/labels"
	labelValuesPath    = "/loki/api/v1/label/{name}/values"
	patternsPath       = "/loki/api/v1/patterns"
)

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
	mux.Handle("/", rp)
	return mux
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
