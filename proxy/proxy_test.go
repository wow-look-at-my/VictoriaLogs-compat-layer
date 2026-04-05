package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/wow-look-at-my/testify/assert"
	"github.com/wow-look-at-my/testify/require"
)

func TestVolumeEndpoint(t *testing.T) {
	// Fake VictoriaLogs backend that responds to /select/logsql/hits.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/select/logsql/hits", r.URL.Path)

		q := r.URL.Query()
		assert.Equal(t, `_stream:{job="test"}`, q.Get("query"))
		assert.NotEmpty(t, q.Get("start"))
		assert.NotEmpty(t, q.Get("end"))
		assert.NotEmpty(t, q.Get("step"))

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"hits":[{"fields":{"job":"test"},"timestamps":["2023-11-14T22:00:00Z"],"values":[42],"total":42}]}`))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet,
		"/loki/api/v1/index/volume?query=%7Bjob%3D%22test%22%7D&start=1700000000000000000&end=1700003600000000000",
		nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp lokiVolumeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, "vector", resp.Data.ResultType)
	require.Equal(t, 1, len(resp.Data.Result))
	assert.Equal(t, "test", resp.Data.Result[0].Metric["job"])
	assert.Equal(t, "42", resp.Data.Result[0].Value[1].(string))
}

func TestVolumeEndpoint_POST(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"hits":[]}`))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	body := strings.NewReader("query=%7Bjob%3D%22test%22%7D&start=1700000000000000000&end=1700003600000000000")
	req := httptest.NewRequest(http.MethodPost, "/loki/api/v1/index/volume", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp lokiVolumeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 0, len(resp.Data.Result))
}

func TestVolumeEndpoint_BackendError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet,
		"/loki/api/v1/index/volume?query=%7Bjob%3D%22test%22%7D&start=1700000000000000000&end=1700003600000000000",
		nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestPassthrough(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "hello from backend: "+r.URL.Path)
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet, "/select/logsql/query?query=*", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "hello from backend")
}

func TestDetectedLabelsEndpoint(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/select/logsql/field_names", r.URL.Path)
		q := r.URL.Query()
		assert.Equal(t, `_stream:{job="test"}`, q.Get("query"))

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"values":[{"value":"job","hits":100},{"value":"level","hits":50}]}`))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet,
		"/loki/api/v1/detected_labels?query=%7Bjob%3D%22test%22%7D&start=1700000000000000000&end=1700003600000000000",
		nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp lokiDetectedLabelsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Status)
	require.Equal(t, 2, len(resp.Data))
	assert.Equal(t, "job", resp.Data[0].Label)
	assert.Equal(t, "S", resp.Data[0].Type)
}

func TestDetectedFieldsEndpoint(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/select/logsql/field_names", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"values":[{"value":"method","hits":200}]}`))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet,
		"/loki/api/v1/detected_fields?query=%7Bjob%3D%22test%22%7D&start=1700000000000000000&end=1700003600000000000",
		nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp lokiDetectedFieldsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.Equal(t, 1, len(resp.Fields))
	assert.Equal(t, "method", resp.Fields[0].Label)
	assert.Equal(t, "string", resp.Fields[0].Type)
}

func TestLabelsEndpoint(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/select/logsql/stream_field_names", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"values":[{"value":"job","hits":100},{"value":"env","hits":50}]}`))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet,
		"/loki/api/v1/labels?start=1700000000000000000&end=1700003600000000000",
		nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp lokiLabelsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, []string{"job", "env"}, resp.Data)
}

func TestLabelValuesEndpoint(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/select/logsql/stream_field_values", r.URL.Path)
		assert.Equal(t, "job", r.URL.Query().Get("field_name"))

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"values":[{"value":"nginx","hits":100},{"value":"app","hits":50}]}`))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet,
		"/loki/api/v1/label/job/values?start=1700000000000000000&end=1700003600000000000",
		nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp lokiLabelsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, []string{"nginx", "app"}, resp.Data)
}

func TestVolumeRangeEndpoint(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/select/logsql/hits", r.URL.Path)
		assert.Equal(t, "5m", r.URL.Query().Get("step"))

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"hits":[{"fields":{"job":"test"},"timestamps":["2023-11-14T22:00:00Z","2023-11-14T23:00:00Z"],"values":[100,250],"total":350}]}`))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet,
		"/loki/api/v1/index/volume_range?query=%7Bjob%3D%22test%22%7D&start=1700000000000000000&end=1700003600000000000&step=5m",
		nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp lokiVolumeRangeResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, "matrix", resp.Data.ResultType)
	require.Equal(t, 1, len(resp.Data.Result))
	assert.Equal(t, "test", resp.Data.Result[0].Metric["job"])
	require.Equal(t, 2, len(resp.Data.Result[0].Values))
}

func TestVolumeRangeEndpoint_BackendError(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet,
		"/loki/api/v1/index/volume_range?query=%7Bjob%3D%22test%22%7D&start=1700000000000000000&end=1700003600000000000",
		nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestPatternsEndpoint(t *testing.T) {
	// No backend needed — patterns returns a static response.
	backendURL, _ := url.Parse("http://localhost:9428")
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet, "/loki/api/v1/patterns", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"success","data":[]}`, rec.Body.String())
}

func TestDetectedLabelsEndpoint_POST(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"values":[]}`))
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	body := strings.NewReader("query=%7Bjob%3D%22test%22%7D&start=1700000000000000000&end=1700003600000000000")
	req := httptest.NewRequest(http.MethodPost, "/loki/api/v1/detected_labels", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp lokiDetectedLabelsResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "success", resp.Status)
	assert.Equal(t, 0, len(resp.Data))
}

func TestVolumeEndpoint_InvalidTimestamp(t *testing.T) {
	backendURL, _ := url.Parse("http://localhost:9428")
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet,
		"/loki/api/v1/index/volume?query=%7Bjob%3D%22test%22%7D&start=invalid&end=1700003600000000000",
		nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
