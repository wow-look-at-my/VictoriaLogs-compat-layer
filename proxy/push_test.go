package proxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/wow-look-at-my/testify/assert"
)

func TestPushEndpoint(t *testing.T) {
	var gotMsgField string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/insert/loki/api/v1/push", r.URL.Path)
		gotMsgField = r.URL.Query().Get("_msg_field")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	body := strings.NewReader(`{"streams":[{"stream":{"job":"test"},"values":[["1700000000000000000","log line"]]}]}`)
	req := httptest.NewRequest(http.MethodPost, "/loki/api/v1/push", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, defaultMsgFields, gotMsgField)
}

func TestPushEndpoint_RespectsClientMsgFieldQuery(t *testing.T) {
	var gotMsgField string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMsgField = r.URL.Query().Get("_msg_field")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodPost, "/loki/api/v1/push?_msg_field=event.original", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, "event.original", gotMsgField)
}

func TestPushEndpoint_RespectsClientMsgFieldHeader(t *testing.T) {
	var gotMsgField string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMsgField = r.URL.Query().Get("_msg_field")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodPost, "/loki/api/v1/push", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("VL-Msg-Field", "event.original")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Empty(t, gotMsgField, "client supplied VL-Msg-Field; query default should not be added")
}

func TestPromPushEndpoint(t *testing.T) {
	var gotMsgField string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/insert/loki/api/v1/push", r.URL.Path)
		gotMsgField = r.URL.Query().Get("_msg_field")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodPost, "/api/prom/push", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, defaultMsgFields, gotMsgField)
}

func TestOTLPLogsEndpoint(t *testing.T) {
	var gotMsgField string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/insert/opentelemetry/api/logs/export", r.URL.Path)
		gotMsgField = r.URL.Query().Get("_msg_field")
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodPost, "/otlp/v1/logs", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, defaultMsgFields, gotMsgField)
}
