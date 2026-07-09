package proxy

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/wow-look-at-my/testify/assert"
	"github.com/wow-look-at-my/testify/require"
)

func TestTailEndpoint_NonWebSocket(t *testing.T) {
	backendURL, _ := url.Parse("http://localhost:9428")
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet, "/loki/api/v1/tail", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUpgradeRequired, rec.Code)
}

func TestPromTailEndpoint_NonWebSocket(t *testing.T) {
	backendURL, _ := url.Parse("http://localhost:9428")
	handler := NewProxy(backendURL)

	req := httptest.NewRequest(http.MethodGet, "/api/prom/tail", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUpgradeRequired, rec.Code)
}

// dialWS dials a WebSocket connection to the given server address and path.
// It returns the connection and a bufio.Reader wrapping it (to handle buffering
// from the HTTP response). The 101 response headers are consumed before returning.
func dialWS(t *testing.T, addr, path string) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	require.NoError(t, err)

	key := base64.StdEncoding.EncodeToString([]byte("test-ws-key-12345"))
	rawReq := "GET " + path + " HTTP/1.1\r\n" +
		"Host: " + addr + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n\r\n"
	_, err = conn.Write([]byte(rawReq))
	require.NoError(t, err)

	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	require.NoError(t, err)
	assert.Contains(t, statusLine, "101", "expected 101 Switching Protocols")
	for {
		line, err := br.ReadString('\n')
		require.NoError(t, err)
		if strings.TrimSpace(line) == "" {
			break
		}
	}
	return conn, br
}

// wsReadTextFrame reads one WebSocket text frame from r (handles short payloads only).
func wsReadTextFrame(t *testing.T, r io.Reader) []byte {
	t.Helper()
	hdr := make([]byte, 2)
	_, err := io.ReadFull(r, hdr)
	require.NoError(t, err)

	opcode := hdr[0] & 0x0f
	assert.Equal(t, byte(1), opcode, "expected text frame (opcode 1)")

	length := int(hdr[1] & 0x7f)
	if length == 126 {
		ext := make([]byte, 2)
		_, err = io.ReadFull(r, ext)
		require.NoError(t, err)
		length = int(ext[0])<<8 | int(ext[1])
	}

	payload := make([]byte, length)
	_, err = io.ReadFull(r, payload)
	require.NoError(t, err)
	return payload
}

func TestTailEndpoint_WebSocket(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/select/logsql/tail", r.URL.Path)
		assert.Equal(t, `_stream:{job="test"}`, r.URL.Query().Get("query"))

		flusher, ok := w.(http.Flusher)
		require.True(t, ok, "backend must support Flush")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		w.Write([]byte(`{"_msg":"hello","_time":"2023-11-14T22:13:20Z","_stream":"{job=\"test\"}"}` + "\n"))
		flusher.Flush()
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	srv := httptest.NewServer(NewProxy(backendURL))
	defer srv.Close()

	conn, br := dialWS(t, srv.Listener.Addr().String(),
		"/loki/api/v1/tail?query=%7Bjob%3D%22test%22%7D")
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	payload := wsReadTextFrame(t, br)

	var msg lokiTailMessage
	require.NoError(t, json.Unmarshal(payload, &msg))
	require.Equal(t, 1, len(msg.Streams))
	assert.Equal(t, "test", msg.Streams[0].Stream["job"])
	require.Equal(t, 1, len(msg.Streams[0].Values))
	assert.Equal(t, "hello", msg.Streams[0].Values[0][1])
}

func TestTailEndpoint_MultipleEntries(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		w.WriteHeader(http.StatusOK)

		lines := []string{
			`{"_msg":"line1","_time":"2023-11-14T22:13:20Z","_stream":"{job=\"app\"}"}`,
			`{"_msg":"line2","_time":"2023-11-14T22:13:21Z","_stream":"{job=\"app\"}"}`,
		}
		for _, l := range lines {
			w.Write([]byte(l + "\n"))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer backend.Close()

	backendURL, _ := url.Parse(backend.URL)
	srv := httptest.NewServer(NewProxy(backendURL))
	defer srv.Close()

	conn, br := dialWS(t, srv.Listener.Addr().String(), "/loki/api/v1/tail?query=%7B%7D")
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))

	for _, want := range []string{"line1", "line2"} {
		payload := wsReadTextFrame(t, br)
		var msg lokiTailMessage
		require.NoError(t, json.Unmarshal(payload, &msg))
		require.Equal(t, 1, len(msg.Streams))
		assert.Equal(t, want, msg.Streams[0].Values[0][1])
	}
}

func TestWsWriteText_SmallPayload(t *testing.T) {
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)

	data := []byte("hello world")
	require.NoError(t, wsWriteText(bw, data))

	result := buf.Bytes()
	require.GreaterOrEqual(t, len(result), 2+len(data))
	assert.Equal(t, byte(0x81), result[0])
	assert.Equal(t, byte(len(data)), result[1])
	assert.Equal(t, data, result[2:])
}

func TestWsWriteText_LargePayload(t *testing.T) {
	pr, pw := io.Pipe()
	bw := bufio.NewWriter(pw)

	large := strings.Repeat("x", 200)
	go func() {
		err := wsWriteText(bw, []byte(large))
		assert.NoError(t, err)
		pw.Close()
	}()

	hdr := make([]byte, 4)
	_, err := io.ReadFull(pr, hdr)
	require.NoError(t, err)
	assert.Equal(t, byte(0x81), hdr[0])
	assert.Equal(t, byte(126), hdr[1])
	length := int(hdr[2])<<8 | int(hdr[3])
	assert.Equal(t, 200, length)

	payload := make([]byte, length)
	_, err = io.ReadFull(pr, payload)
	require.NoError(t, err)
	assert.Equal(t, large, string(payload))
}
