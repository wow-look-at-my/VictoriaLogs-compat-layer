# VictoriaLogs Compat Layer

A reverse proxy that adds Loki API compatibility to [VictoriaLogs](https://docs.victoriametrics.com/victorialogs/).

Point Grafana at this proxy instead of directly at VictoriaLogs. All requests pass through unchanged, except for Loki-specific endpoints that get translated to VictoriaLogs equivalents.

## Supported Endpoints

| Loki Endpoint | VictoriaLogs Equivalent | Description |
|---|---|---|
| `/loki/api/v1/index/volume` | `/select/logsql/hits` | Log volume (instant) |
| `/loki/api/v1/index/volume_range` | `/select/logsql/hits` | Log volume (time series) |
| `/loki/api/v1/detected_labels` | `/select/logsql/field_names` | Discover labels in logs |
| `/loki/api/v1/detected_fields` | `/select/logsql/field_names` | Discover fields in logs |
| `/loki/api/v1/labels` | `/select/logsql/stream_field_names` | List label names |
| `/loki/api/v1/label/{name}/values` | `/select/logsql/stream_field_values` | List values for a label |
| `/loki/api/v1/patterns` | *(stub)* | Returns empty response |

## Usage

```bash
victorialogs-compat-layer -listen :8471 -backend http://localhost:9428
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `-listen` | `:8471` | Address to listen on |
| `-backend` | `http://localhost:9428` | VictoriaLogs backend URL |

### Docker

```bash
docker build -t victorialogs-compat-layer .
docker run -p 8471:8471 victorialogs-compat-layer -backend http://victorialogs:9428
```

## How It Works

1. Grafana sends a Loki API request (e.g., `/loki/api/v1/index/volume`, `/loki/api/v1/detected_labels`)
2. The proxy translates the Loki LogQL stream selector to VictoriaLogs LogSQL (e.g., `{job="foo"}` becomes `_stream:{job="foo"}`)
3. Nanosecond epoch timestamps are converted to RFC3339
4. The request is forwarded to the corresponding VictoriaLogs endpoint
5. The response is converted from VictoriaLogs format to the expected Loki response format
6. All other requests are proxied to VictoriaLogs unchanged

## Development

Build and test with [go-toolchain](https://github.com/wow-look-at-my/go-toolchain):

```bash
go-toolchain
```
