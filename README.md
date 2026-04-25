# VictoriaLogs Compat Layer

A reverse proxy that adds Loki API compatibility to [VictoriaLogs](https://docs.victoriametrics.com/victorialogs/).

Point Grafana (or any Loki client) at this proxy instead of directly at VictoriaLogs. All requests pass through unchanged, except for Loki-specific endpoints that get translated to VictoriaLogs equivalents.

## Supported Endpoints

### Query & discovery

| Loki Endpoint | VictoriaLogs Equivalent | Description |
|---|---|---|
| `/loki/api/v1/query` | `/select/logsql/query` | Instant query |
| `/loki/api/v1/query_range` | `/select/logsql/query` | Range query |
| `/loki/api/v1/series` | `/select/logsql/streams` | List matching series |
| `/loki/api/v1/labels` | `/select/logsql/stream_field_names` | List label names |
| `/loki/api/v1/label` | `/select/logsql/stream_field_names` | Alias of `/labels` |
| `/loki/api/v1/label/{name}/values` | `/select/logsql/stream_field_values` | List values for a label |
| `/loki/api/v1/detected_labels` | `/select/logsql/field_names` | Discover labels |
| `/loki/api/v1/detected_fields` | `/select/logsql/field_names` | Discover fields |
| `/loki/api/v1/detected_field/{name}/values` | `/select/logsql/field_values` | Values for a detected field |
| `/loki/api/v1/index/volume` | `/select/logsql/hits` | Log volume (single bucket) |
| `/loki/api/v1/index/volume_range` | `/select/logsql/hits` | Log volume (time series) |
| `/loki/api/v1/index/stats` | `/select/logsql/stats` | Index statistics |
| `/loki/api/v1/tail` | `/select/logsql/tail` | WebSocket live tail |

### Ingest

| Loki Endpoint | VictoriaLogs Equivalent |
|---|---|
| `/loki/api/v1/push` | `/insert/loki/api/v1/push` |
| `/otlp/v1/logs` | `/insert/opentelemetry/api/logs/export` |

If the request doesn't already specify `_msg_field` (query) or `VL-Msg-Field` (header), the proxy adds `_msg_field=_msg,message,msg,body,log,event,event.original` so VictoriaLogs can find the log message in the common JSON shapes used by structured loggers, OTLP, and Docker/K8s — preventing the "missing _msg field" warning. Clients that pass either argument keep their explicit choice.

### Prometheus-style aliases

The legacy `/api/prom/*` variants (`query`, `label`, `label/{name}/values`, `series`, `push`, `tail`) are accepted and dispatched to the same handlers as their `/loki/api/v1/*` equivalents.

### Stubs

| Endpoint | Behavior |
|---|---|
| `/loki/api/v1/patterns` | Returns `{"status":"success","data":[]}` |
| `/loki/api/v1/status/buildinfo` | Returns a static Loki build info payload |
| `/ready`, `/healthz` | Return 200 OK |

### Not implemented

The following return `501 Not Implemented` explicitly (rather than confusingly proxying through):

- `/loki/api/v1/index/shards`
- Ruler / alerting: `/loki/api/v1/rules[...]`, `/prometheus/api/v1/rules`, `/prometheus/api/v1/alerts`, `/api/prom/rules[...]`
- Log deletion: `/loki/api/v1/delete`, `/loki/api/v1/cache/generation_numbers`

All other paths are proxied through to VictoriaLogs unchanged.

## Usage

```bash
victorialogs-compat-layer -listen :8471 -backend http://localhost:9428
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `-listen` | `:8471` | Address to listen on |
| `-backend` | `http://localhost:9428` | VictoriaLogs backend URL |

### Authentication

`Authorization` and `X-Scope-OrgID` headers from the incoming request are forwarded to the VictoriaLogs backend.

### Docker

```bash
docker build -t victorialogs-compat-layer .
docker run -p 8471:8471 victorialogs-compat-layer -backend http://victorialogs:9428
```

A prebuilt image is also published to GHCR via the repo's Docker workflow.

## How It Works

1. Grafana sends a Loki API request (e.g., `/loki/api/v1/query_range`, `/loki/api/v1/index/volume`).
2. The proxy translates the Loki LogQL stream selector to VictoriaLogs LogSQL (e.g., `{job="foo"}` becomes `_stream:{job="foo"}`).
3. Timestamps are normalized — both nanosecond Unix epochs and RFC3339 / ISO 8601 strings are accepted and converted to RFC3339 for VictoriaLogs.
4. The request is forwarded to the corresponding VictoriaLogs endpoint with auth headers preserved.
5. The VictoriaLogs response is converted back into the Loki response shape the client expects.
6. Live tail (`/loki/api/v1/tail`) is handled as a WebSocket: the proxy opens a streaming connection to `/select/logsql/tail` and re-frames entries into Loki's tail message format.
7. All other requests are proxied to VictoriaLogs unchanged.

## Development

Build and test with [go-toolchain](https://github.com/wow-look-at-my/go-toolchain):

```bash
go-toolchain
```

See `API_INVENTORY.md` for the tracking checklist of Loki API surface coverage.
