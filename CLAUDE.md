# VictoriaLogs Compat Layer

## Project Overview

A Go reverse proxy that translates Loki API endpoints to VictoriaLogs equivalents, enabling Grafana features (like Drilldown) that depend on Loki-specific APIs.

## Structure

- `main.go` — Entry point, CLI flag parsing, server startup
- `proxy/proxy.go` — Reverse proxy with Loki endpoint interception
- `proxy/translate.go` — Loki-to-VictoriaLogs request translation (query, timestamps, parameters)
- `proxy/response.go` — VictoriaLogs-to-Loki response format conversion
- `Dockerfile` — Multi-stage Docker build

## Key Details

- **No external dependencies** beyond `github.com/wow-look-at-my/testify` (test only)
- Uses Go stdlib `net/http/httputil.ReverseProxy` for pass-through
- Intercepts `/loki/api/v1/index/volume` → translates to `/select/logsql/hits`
- All other paths proxied unchanged to VictoriaLogs backend

## Build & Test

```bash
go-toolchain
```

Do NOT use bare `go build`, `go test`, or `go run`. Always use `go-toolchain`.
