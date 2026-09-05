# Implemented foundation configuration

Source of truth: `internal/config.Values`, `Defaults()` and validation. JSON only; unknown, retired, duplicate, null and trailing documents are rejected. Document size is at most 1 MiB and nesting at most 32 levels. Errors identify a safe known field/container and fixed rule, never a rejected value or secret environment contents. The full application/provider APIs remain in later phases.

Use `chartworks config-check --defaults` for a machine-readable defaults snapshot. Required blanks deliberately do not form a runnable development-authority configuration. The example `examples/chartworks.foundation.json` supplies references for the required deployment-specific values.

## Transport and metadata storage

| Key | Type / units | Default | Bounds and behavior |
|---|---|---|---|
| `server.listen` | host:port string | `127.0.0.1:8080` | Explicit loopback IP only in this foundation; numeric port 0–65535. Port 0 is useful for isolated tests. |
| `server.read_header_timeout` | duration string | `5s` | Positive, at most 1 minute. |
| `server.read_timeout` | duration string | `15s` | Positive, at most 5 minutes. |
| `server.write_timeout` | duration string | `30s` | Positive, at most 5 minutes. |
| `server.idle_timeout` | duration string | `1m0s` | Positive, at most 10 minutes. |
| `server.shutdown_grace` | duration string | `10s` | Positive, at most 1 minute; timeout forces HTTP closure. |
| `server.max_body_bytes` | integer bytes | 10 MiB | 1 byte–100 MiB. Health endpoints accept no body; oversized known bodies receive 413. |
| `server.max_header_bytes` | integer bytes | 32 KiB | 1 KiB–1 MiB; enforced by Go HTTP server. |
| `store.dsn` | secret reference | `env:CHARTWORKS_STORE_URL` | Required nonblank environment value, never a literal credential in configuration. No implicit ambient database connection. |
| `store.max_conns` | integer | 10 | 1–100; minimum idle connections is zero. |
| `store.connect_timeout` | duration string | `5s` | Positive, at most 1 minute; initial connection/ping is bounded. |
| `store.transaction_timeout` | duration string | `5s` | At least 1 millisecond, at most 1 minute; context deadline plus PostgreSQL statement/lock limits. |
| `store.migration_policy` | enum | `apply` | `apply` atomically adds pending forward migrations; `check` rejects missing/mismatched history without migrating. |

The PostgreSQL schema is `chartworks`; queries are schema-qualified and pooled sessions use `search_path=pg_catalog`. The executable never accepts arbitrary migration paths or raw query strings from an HTTP caller. Warehouse-source connection configuration is not implemented by these metadata settings.

## Verification configuration and dependency health

| Key | Type / units | Default | Bounds and behavior |
|---|---|---|---|
| `auth.issuer` | HTTPS URL or env reference | required | No userinfo, query or fragment; identity issuer belongs to Pengui. |
| `auth.jwks_url` | trusted HTTPS URL or env reference | required | No userinfo, query or fragment; redirects refused. HTTP fetch timeout and 1 MiB/32-key response limits. |
| `auth.audience` | string or env reference | required | Nonempty, at most 512 bytes; no spaces/control separators. Exact intended audience is reserved for phase 03 enforcement. |
| `auth.algorithms` | string array | `RS256`, `ES256` | Nonempty unique allowlist from RS/ES 256/384/512. HS/none unsupported. |
| `auth.jwks_max_stale` | duration | `5m0s` | Positive, at most 1 hour. Failed refresh cannot reset last-success expiry. |
| `auth.refresh_interval` | duration | `1m0s` | Positive and strictly below maximum stale age. |
| `auth.request_timeout` | duration | `3s` | Positive, at most 1 minute. |
| `auth.clock_skew` | duration | `30s` | 0–1 minute; validated/reserved for the phase 03 verifier, not used to extend the health probe's key freshness. |
| `auth.max_token_lifetime` | duration | `15m0s` | Positive, at most 24 hours; validated/reserved for phase 03. No token is accepted by this foundation. |

The key-health probe rejects duplicate IDs/fields, symmetric/private key material, invalid public coordinates/moduli and incompatible declared algorithms. It does not authenticate requests. Phase 03 must consume the same health seam with its real verifier/cache, not mistake the probe for JWT verification or add a local issuer.

## Telemetry and explicit capability enablement

| Key | Type | Default | Behavior |
|---|---|---|---|
| `telemetry.log_format` | enum | `json` | `json` or `text`, using slog only. |
| `telemetry.metrics` | boolean | true | Enables the internal counter/gauge exporter. False disables updates/export while retaining bounded lifecycle logs. The exporter is not mounted on the public/health router. |
| `telemetry.otel` | boolean | false | True is explicitly rejected: the optional export adapter is not implemented, not silently ignored. |
| `features.gateway` | boolean | false | True rejected until phase 05 is implemented. |
| `features.mcp` | boolean | false | True rejected until the MCP surface phase is implemented. |
| `features.reporting` | boolean | false | True rejected until reporting phases are implemented. |
| `features.renderer` | boolean | false | True rejected until rendering is implemented. |

`/capabilities` reports implemented foundation functionality and explicitly says business API and authentication are unavailable. It never returns provider credentials, source IDs or deployment DSNs. `/metrics`, `/mcp`, `/v1/*`, login and bootstrap paths are absent. Later authenticated API registration owns protected metric/audit access.

## Future Bifrost configuration accepted as an inactive excerpt

`gateway.driver` must be `bifrost`; `gateway.max_attempts_per_call` defaults to 2 and accepts 1–4. `gateway.bifrost.providers[]` contains a unique remote `name`, an `api_key` in `env:NAME` form, and an optional HTTPS `base_url` without userinfo/query/fragment. No local/ollama/mock production provider is accepted. Provider credentials are not resolved or used while the capability is unimplemented.

`gateway.roles` is a closed map: `embedding`, `enhance`, `sqlgen`, `sqlfix`, `clarify`, `pipeline_draft`, `profile_summary`, `rerank`, `narrative`, `visual_rank`. Each configured role supplies its remote provider/model and a positive timeout of at most 5 minutes. `max_tokens` is 0–65536; phase 05 applies the role-specific execution budget. Optional roles can carry `enabled` and `on_failure` (`fail` or `preserve_candidates`).

The embedding role also requires `dimensions` 1–16384, `max_batch_items` 1–1024 and `max_batch_bytes` 1–4 MiB. Rerank requires `max_candidates` 1–1024. These are configuration-shape bounds, not live provider capability proofs. The separate [gateway contract](contracts/model-gateway.md) owns execution/response validation in phase 05. The existing example remains the remote embedding/rerank starting point; no local model support is added.

## Sources and secret handling

File selection: explicit `--config` takes precedence over `CHARTWORKS_CONFIG`. Value precedence: typed defaults -> supplied file fields -> explicit named environment references -> explicit `--listen`. Environment names after `env:` use uppercase letters, digits after the first character, and underscores, with a 128-character ceiling. Missing or empty referenced values fail. There is no second flat environment alias system.

Configuration snapshots and their returned `Values()` copies are safe to share concurrently. Printing the configuration redacts resolved credentials; serialization includes references only. The raw DSN accessor is limited to connection construction and must never be logged. All driver failures are mapped to safe categories rather than returning SQL/connection error text.
