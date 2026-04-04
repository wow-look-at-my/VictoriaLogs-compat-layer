# VictoriaLogs Compat Layer

A reverse proxy that adds Loki API compatibility to [VictoriaLogs](https://docs.victoriametrics.com/victorialogs/).

Point Grafana at this proxy instead of directly at VictoriaLogs. All requests pass through unchanged, except for Loki-specific endpoints that get translated to VictoriaLogs equivalents.

## Supported Endpoints

| Loki Endpoint | VictoriaLogs Equivalent | Description |
|---|---|---|
| `/loki/api/v1/index/volume` | `/select/logsql/hits` | Log volume for Grafana drilldown |

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

1. Grafana sends a `/loki/api/v1/index/volume` request (used by the Drilldown plugin)
2. The proxy translates the Loki LogQL stream selector to VictoriaLogs LogSQL (e.g., `{job="foo"}` becomes `_stream:{job="foo"}`)
3. Nanosecond epoch timestamps are converted to RFC3339
4. The request is forwarded to VictoriaLogs' `/select/logsql/hits` endpoint
5. The response is converted from VictoriaLogs' hits format to Loki's Prometheus vector format
6. All other requests are proxied to VictoriaLogs unchanged

## Development

Build and test with [go-toolchain](https://github.com/wow-look-at-my/go-toolchain):

```bash
go-toolchain
```
