package proxy

import (
	"encoding/json"
	"math"
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

// failingBackend returns a backend that fails the test if it ever gets a
// request — used to assert that probe queries never reach VictoriaLogs.
func failingBackend(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("probe query should not reach backend, got request to %s", r.URL.Path)
	}))
}

func TestQueryEndpoint_PromProbe_Vector(t *testing.T) {
	// vector(1)+vector(1) is Grafana's liveness probe — it must evaluate
	// locally to 2 and come back as a Prometheus vector.
	backend := failingBackend(t)
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet,
		"/loki/api/v1/query?query=vector(1)%2Bvector(1)&time=1700000000000000000",
		nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp lokiVolumeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, "vector", resp.Data.ResultType)
	require.Equal(t, 1, len(resp.Data.Result))
	assert.Equal(t, "2", resp.Data.Result[0].Value[1])
	assert.InDelta(t, 1700000000.0, resp.Data.Result[0].Value[0], 1)
}

func TestQueryRangeEndpoint_PromProbe_Matrix(t *testing.T) {
	backend := failingBackend(t)
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet,
		"/loki/api/v1/query_range?query=vector(1)%2Bvector(1)&start=1700000000000000000&end=1700003600000000000&step=600s",
		nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp lokiVolumeRangeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, "matrix", resp.Data.ResultType)
	require.Equal(t, 1, len(resp.Data.Result))
	require.Greater(t, len(resp.Data.Result[0].Values), 1, "matrix should have multiple samples")
	for _, v := range resp.Data.Result[0].Values {
		assert.Equal(t, "2", v[1])
	}
}

func TestPromQueryEndpoint_PromProbe(t *testing.T) {
	backend := failingBackend(t)
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet,
		"/api/prom/query?query=vector(1)%2Bvector(1)",
		nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp lokiVolumeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "vector", resp.Data.ResultType)
}

func TestQueryEndpoint_UnparseableProbe(t *testing.T) {
	// A probe-shaped query (no '{') we can't parse must still return a 200
	// success with an empty result, never reach the backend.
	backend := failingBackend(t)
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet,
		"/loki/api/v1/query?query=some_unknown_func()",
		nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp lokiVolumeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, "vector", resp.Data.ResultType)
	assert.Equal(t, 0, len(resp.Data.Result))
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

func TestEvalProbe(t *testing.T) {
	tests := []struct {
		q    string
		want float64
		ok   bool
	}{
		{"vector(1)+vector(1)", 2, true},
		{"vector(1)", 1, true},
		{"scalar(7)", 7, true},
		{"1+1", 2, true},
		{"2*3", 6, true},
		{"10/4", 2.5, true},
		{"-5", -5, true},
		{"-(2+3)", -5, true},
		{"(1+2)*3", 9, true},
		{"1.5+2.5", 4, true},
		{"vector(1)+vector(2)*vector(3)", 7, true},
		// Unsupported.
		{"up", 0, false},
		{"some_unknown_func(1)", 0, false},
		{"1/0", 0, false},
		{"1+", 0, false},
		{"(1+2", 0, false},
		{`{job="foo"}`, 0, false},
	}
	for _, tt := range tests {
		got, ok := evalProbe(tt.q)
		assert.Equal(t, tt.ok, ok, "evalProbe(%q) ok", tt.q)
		if tt.ok {
			assert.True(t, math.Abs(got-tt.want) < 1e-9, "evalProbe(%q) = %v, want %v", tt.q, got, tt.want)
		}
	}
}
