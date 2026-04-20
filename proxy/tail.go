package proxy

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"time"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// lokiTailMessage is a single Loki WebSocket tail frame payload.
type lokiTailMessage struct {
	Streams []lokiStreamResult `json:"streams"`
}

// wsUpgrade completes the WebSocket handshake, hijacks w, and returns the
// underlying conn and buffered I/O for subsequent frame writes.
func wsUpgrade(w http.ResponseWriter, r *http.Request) (net.Conn, *bufio.ReadWriter, error) {
	key := r.Header.Get("Sec-Websocket-Key")
	if r.Header.Get("Upgrade") != "websocket" || key == "" {
		http.Error(w, "websocket upgrade required", http.StatusUpgradeRequired)
		return nil, nil, fmt.Errorf("not a websocket request")
	}

	h := sha1.New()
	h.Write([]byte(key + wsGUID))
	accept := base64.StdEncoding.EncodeToString(h.Sum(nil))

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return nil, nil, fmt.Errorf("hijacking not supported")
	}

	conn, brw, err := hijacker.Hijack()
	if err != nil {
		return nil, nil, err
	}

	_, err = fmt.Fprintf(brw,
		"HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: %s\r\n\r\n",
		accept)
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	if err = brw.Flush(); err != nil {
		conn.Close()
		return nil, nil, err
	}

	return conn, brw, nil
}

// wsWriteText writes an unmasked text WebSocket frame and flushes.
func wsWriteText(w *bufio.Writer, data []byte) error {
	n := len(data)
	switch {
	case n <= 125:
		if _, err := w.Write([]byte{0x81, byte(n)}); err != nil {
			return err
		}
	case n <= 65535:
		if _, err := w.Write([]byte{0x81, 126, byte(n >> 8), byte(n & 0xff)}); err != nil {
			return err
		}
	default:
		hdr := make([]byte, 10)
		hdr[0] = 0x81
		hdr[1] = 127
		for i := 0; i < 8; i++ {
			hdr[9-i] = byte(n >> uint(8*i))
		}
		if _, err := w.Write(hdr); err != nil {
			return err
		}
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	return w.Flush()
}

// handleTail upgrades the connection to WebSocket and streams VictoriaLogs
// tail entries as Loki-format frames until the backend closes or write fails.
func handleTail(w http.ResponseWriter, r *http.Request, backend *url.URL) {
	params, err := extractParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	conn, brw, err := wsUpgrade(w, r)
	if err != nil {
		return
	}
	defer conn.Close()

	vlReq, err := BuildQueryRequest(backend, params)
	if err != nil {
		log.Printf("tail: build request: %v", err)
		return
	}
	vlReq.URL.Path = "/select/logsql/tail"

	vlResp, err := http.DefaultClient.Do(vlReq)
	if err != nil {
		log.Printf("tail: backend: %v", err)
		return
	}
	defer vlResp.Body.Close()

	scanner := bufio.NewScanner(vlResp.Body)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var entry vlQueryEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		t, err := time.Parse(time.RFC3339Nano, entry.Time)
		if err != nil {
			if t, err = time.Parse(time.RFC3339, entry.Time); err != nil {
				continue
			}
		}

		msg := lokiTailMessage{
			Streams: []lokiStreamResult{
				{
					Stream: parseStreamSelector(entry.Stream),
					Values: [][2]string{{fmt.Sprintf("%d", t.UnixNano()), entry.Msg}},
				},
			},
		}

		data, err := json.Marshal(msg)
		if err != nil {
			continue
		}

		if err := wsWriteText(brw.Writer, data); err != nil {
			return
		}
	}
}
