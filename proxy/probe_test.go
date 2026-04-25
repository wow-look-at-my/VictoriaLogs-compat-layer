package proxy

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/wow-look-at-my/testify/assert"
	"github.com/wow-look-at-my/testify/require"
)

func TestDrilldownLimitsEndpoint(t *testing.T) {
	// No backend needed — drilldown-limits returns a static response.
	backendURL, _ := url.Parse("http://localhost:9428")
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet, "/loki/api/v1/drilldown-limits", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var limits map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &limits))
	assert.NotNil(t, limits["max_query_series"])
	assert.NotEmpty(t, limits["max_query_length"])
	assert.NotEmpty(t, limits["max_query_lookback"])
}

func TestQueryEndpoint_PromProbe(t *testing.T) {
	// Synthetic Prometheus probe queries (no stream selector) must not reach
	// the backend — they should be short-circuited with an empty success response.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("probe query should not reach backend, got request to %s", r.URL.Path)
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet,
		"/loki/api/v1/query?query=vector(1)%2Bvector(1)",
		nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp lokiQueryResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, "streams", resp.Data.ResultType)
	assert.Equal(t, 0, len(resp.Data.Result))
}

func TestQueryRangeEndpoint_PromProbe(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("probe query should not reach backend, got request to %s", r.URL.Path)
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet,
		"/loki/api/v1/query_range?query=vector(1)%2Bvector(1)&start=1700000000000000000&end=1700003600000000000",
		nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp lokiQueryResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, 0, len(resp.Data.Result))
}

func TestPromQueryEndpoint_PromProbe(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("probe query should not reach backend, got request to %s", r.URL.Path)
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet,
		"/api/prom/query?query=vector(1)%2Bvector(1)",
		nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestIsProbeQuery(t *testing.T) {
	tests := []struct {
		q    string
		want bool
	}{
		{"", false},
		{"   ", false},
		{`{job="foo"}`, false},
		{`{job="foo"} |= "error"`, false},
		{`count_over_time({job="foo"}[5m])`, false},
		{"vector(1)+vector(1)", true},
		{"1+1", true},
		{"up", true},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, isProbeQuery(tt.q), "isProbeQuery(%q)", tt.q)
	}
}
