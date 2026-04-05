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
- Intercepts Loki API paths and translates them to VictoriaLogs equivalents:
  - `/loki/api/v1/index/volume` → `/select/logsql/hits` (single-bucket volume)
  - `/loki/api/v1/index/volume_range` → `/select/logsql/hits` (time-series volume)
  - `/loki/api/v1/detected_labels` → `/select/logsql/field_names`
  - `/loki/api/v1/detected_fields` → `/select/logsql/field_names`
  - `/loki/api/v1/labels` → `/select/logsql/stream_field_names`
  - `/loki/api/v1/label/{name}/values` → `/select/logsql/stream_field_values`
  - `/loki/api/v1/patterns` → returns empty stub response
- All other paths proxied unchanged to VictoriaLogs backend

## Build & Test

```bash
go-toolchain
```

Do NOT use bare `go build`, `go test`, or `go run`. Always use `go-toolchain`.
