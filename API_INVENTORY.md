# Loki API Surface Inventory

Tracks every Loki HTTP endpoint against its current status in the compat layer.

**Legend**
- ✅ Translated — compat layer intercepts and converts to a VictoriaLogs equivalent
- 🔁 Pass-through — forwarded to VictoriaLogs, which supports it natively
- 🔲 Missing — not handled; proxied to VL which returns an error or wrong response
- 🪄 Stub — intercepted and returns a hardcoded/empty response

---

## Query API

The core read path. Used by Grafana Explore, dashboards, and logcli.

| Method | Path | Status | VictoriaLogs equivalent | Notes |
|--------|------|--------|------------------------|-------|
| GET/POST | `/loki/api/v1/query_range` | 🔲 Missing | `/select/logsql/query` | Range query — the most-used Grafana endpoint |
| GET/POST | `/loki/api/v1/query` | 🔲 Missing | `/select/logsql/query` | Instant query |

---

## Push / Ingest API

| Method | Path | Status | VictoriaLogs equivalent | Notes |
|--------|------|--------|------------------------|-------|
| POST | `/loki/api/v1/push` | 🔲 Missing | `/insert/loki/api/v1/push` | VL path prefix differs from Loki |
| POST | `/api/prom/push` | 🔲 Missing | `/insert/loki/api/v1/push` | Legacy push path |
| POST | `/otlp/v1/logs` | 🔲 Missing | `/insert/opentelemetry/api/logs/export` | OTLP ingest |

---

## Label / Metadata API

| Method | Path | Status | VictoriaLogs equivalent | Notes |
|--------|------|--------|------------------------|-------|
| GET/POST | `/loki/api/v1/labels` | ✅ Translated | `/select/logsql/stream_field_names` | |
| GET/POST | `/loki/api/v1/label` | 🔲 Missing | `/select/logsql/stream_field_names` | Alias for `/labels`; separate constant in Loki |
| GET/POST | `/loki/api/v1/label/{name}/values` | ✅ Translated | `/select/logsql/stream_field_values` | |
| GET/POST | `/loki/api/v1/series` | 🔲 Missing | `/select/logsql/streams` | Returns matching log streams |

---

## Detected Labels / Fields (Drilldown)

Used by Grafana Logs Drilldown.

| Method | Path | Status | VictoriaLogs equivalent | Notes |
|--------|------|--------|------------------------|-------|
| GET/POST | `/loki/api/v1/detected_labels` | ✅ Translated | `/select/logsql/field_names` | |
| GET/POST | `/loki/api/v1/detected_fields` | ✅ Translated | `/select/logsql/field_names` | |
| GET/POST | `/loki/api/v1/detected_field/{name}/values` | 🔲 Missing | `/select/logsql/field_values` | Per-field value enumeration |

---

## Index API

| Method | Path | Status | VictoriaLogs equivalent | Notes |
|--------|------|--------|------------------------|-------|
| GET/POST | `/loki/api/v1/index/volume` | ✅ Translated | `/select/logsql/hits` | Single-bucket volume |
| GET/POST | `/loki/api/v1/index/volume_range` | ✅ Translated | `/select/logsql/hits` | Time-series volume |
| GET/POST | `/loki/api/v1/index/stats` | 🔲 Missing | `/select/logsql/stats` | Byte/chunk/entry counts |
| GET/POST | `/loki/api/v1/index/shards` | 🔲 Missing | *(no VL equivalent)* | Query sharding hint; can return stub |

---

## Patterns

| Method | Path | Status | VictoriaLogs equivalent | Notes |
|--------|------|--------|------------------------|-------|
| GET/POST | `/loki/api/v1/patterns` | 🪄 Stub | *(none)* | Returns `{"status":"success","data":[]}` |

---

## Live Tail (Streaming)

| Method | Path | Status | VictoriaLogs equivalent | Notes |
|--------|------|--------|------------------------|-------|
| GET | `/loki/api/v1/tail` | 🔲 Missing | `/select/logsql/tail` | WebSocket; VL tail uses different framing |
| GET | `/api/prom/tail` | 🔲 Missing | `/select/logsql/tail` | Legacy WebSocket tail |

---

## Ruler / Alerting

VictoriaLogs has no native ruler. These endpoints are typically handled by a separate recording-rules component (e.g., `vmalert`). Mark as intentionally out-of-scope or stub 404 so clients fail cleanly.

| Method | Path | Status | Notes |
|--------|------|--------|-------|
| GET | `/loki/api/v1/rules` | 🔲 Missing | List all rule groups |
| GET | `/loki/api/v1/rules/{namespace}` | 🔲 Missing | |
| POST | `/loki/api/v1/rules/{namespace}` | 🔲 Missing | Create/update rule group |
| DELETE | `/loki/api/v1/rules/{namespace}` | 🔲 Missing | |
| GET | `/loki/api/v1/rules/{namespace}/{groupName}` | 🔲 Missing | |
| DELETE | `/loki/api/v1/rules/{namespace}/{groupName}` | 🔲 Missing | |
| GET | `/prometheus/api/v1/rules` | 🔲 Missing | Prometheus-compat rule list |
| GET | `/prometheus/api/v1/alerts` | 🔲 Missing | Prometheus-compat alert list |
| GET | `/api/prom/rules` | 🔲 Missing | Legacy |
| POST | `/api/prom/rules/{namespace}` | 🔲 Missing | Legacy |
| DELETE | `/api/prom/rules/{namespace}` | 🔲 Missing | Legacy |
| GET | `/api/prom/rules/{namespace}/{groupName}` | 🔲 Missing | Legacy |
| DELETE | `/api/prom/rules/{namespace}/{groupName}` | 🔲 Missing | Legacy |

---

## Log Deletion (Compactor)

| Method | Path | Status | Notes |
|--------|------|--------|-------|
| PUT/POST | `/loki/api/v1/delete` | 🔲 Missing | Add deletion request |
| GET | `/loki/api/v1/delete` | 🔲 Missing | List deletion requests |
| DELETE | `/loki/api/v1/delete` | 🔲 Missing | Cancel deletion request |
| GET | `/loki/api/v1/cache/generation_numbers` | 🔲 Missing | Cache invalidation; can stub |

---

## Legacy Prometheus-Compat Query API (`/api/prom/*`)

Older Grafana datasource plugin versions use this path prefix.

| Method | Path | Status | VictoriaLogs equivalent | Notes |
|--------|------|--------|------------------------|-------|
| GET/POST | `/api/prom/query` | 🔲 Missing | `/select/logsql/query` | Same semantics as `/loki/api/v1/query` |
| GET/POST | `/api/prom/label` | 🔲 Missing | `/select/logsql/stream_field_names` | |
| GET/POST | `/api/prom/label/{name}/values` | 🔲 Missing | `/select/logsql/stream_field_values` | |
| GET/POST | `/api/prom/series` | 🔲 Missing | `/select/logsql/streams` | |

---

## Management / Health

| Method | Path | Status | VictoriaLogs equivalent | Notes |
|--------|------|--------|------------------------|-------|
| GET | `/healthz` | 🪄 Stub | — | Returns 200; custom compat-layer endpoint |
| GET | `/ready` | 🔲 Missing | `/health` | Standard Kubernetes readiness probe path |
| GET | `/metrics` | 🔁 Pass-through | `/metrics` | VL serves Prometheus metrics natively |
| GET | `/loki/api/v1/status/buildinfo` | 🔲 Missing | *(none)* | Return stub Loki version JSON so clients don't break |

---

## Summary

| Status | Count |
|--------|-------|
| ✅ Translated | 6 |
| 🪄 Stub | 2 |
| 🔁 Pass-through | 1 |
| 🔲 Missing | 35 |

### Recommended implementation order

1. **`/loki/api/v1/query_range`** and **`/loki/api/v1/query`** — without these, Grafana dashboards and Explore show no data
2. **`/loki/api/v1/push`** — without this, no logs can be ingested via the compat layer
3. **`/loki/api/v1/series`** — required by Grafana label browser
4. **`/loki/api/v1/label`** (alias) — Grafana sometimes hits this instead of `/labels`
5. **`/loki/api/v1/detected_field/{name}/values`** — Drilldown completeness
6. **`/loki/api/v1/index/stats`** — used for query planning display in Grafana
7. **`/api/prom/*`** legacy block — needed for older datasource plugin versions
8. **`/loki/api/v1/tail`** + `/api/prom/tail` — live tail support
9. **`/ready`** + **`/loki/api/v1/status/buildinfo`** — Kubernetes and client compatibility stubs
10. **`/otlp/v1/logs`** — OTLP ingest path
11. **Ruler** and **deletion** endpoints — low priority; stub with 501 for now
