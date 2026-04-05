package proxy

import (
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

const volumePath = "/loki/api/v1/index/volume"

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
	mux.HandleFunc(volumePath, func(w http.ResponseWriter, r *http.Request) {
		handleVolume(w, r, backend)
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
