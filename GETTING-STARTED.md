# Running Chartworks phases 01–06

This build implements configuration, lifecycle/health, PostgreSQL metadata, Pengui JWT verification and signed-scope enforcement on its operational APIs. It also implements remote Bifrost inference and durable maintenance scheduling. It does **not** implement NLQ, reporting, the full MCP server or rendering yet. Pengui remains the sole authority issuer; Chartworks creates no credentials or local identity policy.

## Requirements

Use the pinned Go 1.26.4 toolchain, Python 3.10+ for repository checks, Docker Compose for disposable PostgreSQL, and PostgreSQL 17 client tools (`pg_dump`, `pg_restore`, `psql`) for backup/restore tests. The test account must have permission to create and drop disposable databases. Never point `CHARTWORKS_TEST_STORE_URL` at a customer warehouse.

```bash
make pg-up
export CHARTWORKS_TEST_STORE_URL='postgres://chartworks:chartworks@localhost:5434/chartworks?sslmode=disable'
export CHARTWORKS_STORE_URL="$CHARTWORKS_TEST_STORE_URL"
make build
./bin/chartworks version
```

The password shown above is the documented Docker-only fixture, not a production credential. Production connections require an operator-supplied DSN, transport encryption appropriate to the deployment, and a dedicated Chartworks metadata database. Customer source data never goes into this metadata schema.

## Configure and run

Use Pengui's configured issuer, public JWKS URL and intended capability audience. These are not signing secrets. Missing values are configuration errors; no first-user or developer fallback exists.

```bash
export CHARTWORKS_ISSUER='https://your-pengui-host.example'
export CHARTWORKS_JWKS_URL='https://your-pengui-host.example/.well-known/jwks.json'
export CHARTWORKS_AUDIENCE='your-chartworks-audience'
./bin/chartworks config-check --config examples/chartworks.foundation.json
./bin/chartworks serve --config examples/chartworks.foundation.json
```

Replace the example host and audience with the real platform configuration. `config-check` performs no network calls or inference. `serve` connects PostgreSQL and applies/checks migrations before listening. The foundation binds **only an explicit loopback IP**; broad listeners and external TLS deployment remain with the wider transport phase. JWT verification is active on every registered operational endpoint. Do not publish this foundation through an unauthenticated reverse proxy.

```bash
curl --fail http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
curl --fail http://127.0.0.1:8080/capabilities
```

Liveness is independent of dependency health. Readiness reports `starting`, `ready`, `unavailable` or `stale` for PostgreSQL and trusted verification-key material. The real JWT verifier and readiness use the same bounded trusted public-key cache; a readiness result does not authorize an individual request. Failed key refresh never extends key freshness. The default configuration does not enable inference or workers, and readiness never makes a paid model call.

Use Ctrl+C or SIGTERM to drain the listener, cancel and join dependency monitors, release idle HTTP connections and close the database pool. `chartworks mcp` currently exits 3 with an explicit unavailable message; it does not start a fake MCP server. Operational `/v1/*` and `/metrics` requests now require a valid Pengui bearer plus their separately registered action and addressed reach. Anonymous requests return 401; a valid caller receives a nondisclosing 404 for unregistered/inaccessible resources. Metrics also needs `ops.metrics` and returns 404 when disabled. Use the operation manifest below; there is no local login or default administrator token.

## Configuration contract

JSON is the implemented format. `config-check --defaults` prints the exact typed defaults, with secret references rather than resolved credentials. Precedence is defaults -> file -> explicitly named `env:NAME` references -> explicit `--listen` override. `--config` selects the file before `CHARTWORKS_CONFIG`; arbitrary environment variables do not silently override fields. See [the complete key reference](docs/configuration.md).

The `examples/chartworks.gateway.json` excerpt is now implemented. Combine it with your foundation configuration and set `features.gateway=true` to construct the remote SDK clients. Provider secrets are resolved at service construction, not by `config-check`; construction makes no inference calls. No local model download or second inference service is required.

## Tests and completion

```bash
make planning-check
make test
make coverage
make vet
make lint
python3 scripts/run_phase_acceptance.py --phase 01
python3 scripts/run_phase_acceptance.py --phase 02
python3 scripts/run_phase_acceptance.py --phase 03
python3 scripts/run_phase_acceptance.py --phase 04
make preflight-full
```

Real-store tests create unique `cw_test_*` databases on the explicit test server and remove them afterward. Missing PostgreSQL/client tools, a missing acceptance child, or a skipped runtime test is a failure, not a pass. Coverage instruments production packages across the full test suite, including integration callers; thresholds remain 85% for store, 80% for other internal code and 70% for CLI.

The other 30 phases remain unimplemented and are explicitly reported as planned skips in development preflight. `make release-check` correctly refuses an all-product release while they are planned. This does not block acceptance of the implemented phases 01–04.

## Backup, restore and rollback

Run the archive tool as an operator against the **dedicated metadata database**, never as a tenant-facing HTTP action. The DSN is expanded into libpq environment fields, never command-line arguments or logs. Backups are created privately with mode 0600 and atomically published without replacing existing files. Treat the backup itself as sensitive data.

```bash
python3 scripts/store_archive.py backup /secure/directory/chartworks.dump
# Provision a NEW empty database and change CHARTWORKS_STORE_URL to that database.
python3 scripts/store_archive.py restore /secure/directory/chartworks.dump --confirm-empty
```

Restore only trusted archives into an empty database under exclusive operator control: PostgreSQL archives can contain executable SQL. Restore is transactional and rejects nonempty targets; it never uses `--clean`. The `--confirm-empty` check is not a lock against another administrator concurrently modifying the target. URI connection options are explicitly supported/validated by the tool; unrecognized options fail rather than disappear.

The driver verifies ordered migration versions and checksums on startup. A changed, missing or future version is a refusal, not an automatic repair. Recovery means restoring a compatible backup into a new database and deliberately switching configuration; never edit an applied migration or run an invented down migration.

The retention service is the first internal consumer of immutable revisions, compare-and-swap pointers, content-free audit, operation keys and fencing. The bounded synchronous sweep is now reachable only through the registered Pengui-JWT-protected operational service; unattended scheduling is not implemented here. Its scope type is a storage isolation coordinate, not authentication. Expired operation keys retain tombstones so retries cannot silently create fresh work. Old revision references and audit dependencies are preserved; a policy change blocks previously accepted destructive work until a new operation is explicitly accepted.

## Verified operational access (phases 03/04)

The production `serve` command now protects retention policy, audit, synchronous retention sweep, diagnostics and metrics with Pengui JWTs and signed addressed scopes. The old health-only foundation boundary is superseded for these implemented operations, not for the later analytical/MCP features. The listener remains explicit-loopback; a trusted backend supplies credentials. See [operator registration](docs/contracts/pengui-provider-registration.md), [operation manifest](docs/contracts/chartworks-operations.json) and [authority contract](docs/contracts/pengui-authority.md). The public Go client is `sdk/chartworks`; its caller supplies a current Pengui token provider. Chartworks issues no credentials.

## Enable remote models

Keep the existing foundation JSON and merge its top-level `gateway` member with `examples/chartworks.gateway.json`; set `features.gateway` to `true`. Get the OpenRouter and Cohere keys from your approved provider accounts and set the environment variables named in the excerpt. The **Cohere key is separate**: the pinned SDK does not support reranking through OpenRouter. Never paste keys into the JSON or commit them.

Review every role's model, timeout and output limits. Choose an operator-owned `embedding.model_revision` and change it whenever the remote embedding generation changes. Dimension equality does not make two spaces interchangeable. Rerank, narrative and visual ranking run only when explicitly enabled. `config-check` validates the choices without spending money.

After startup, an operator with `ops.model` and `cw.tenant.use:<tenant>` can call `POST /v1/gateway/probes` with `{"role":"embedding"}` (or another configured role). This is a **paid remote request with fixed synthetic input**. Use the Go SDK's `ProbeGateway`; it obtains the bearer from your existing Pengui token provider. Do not put a bearer in a URL, command history or a repository file. Probe failures contain sanitized receipts, not provider error bodies. Run one separately authorized live smoke per chosen provider operation before declaring deployment support.

## Enable durable maintenance

First merge and deploy the companion Pengui execution-authority change and configure its operator-approved binding file, described in [execution authority v1](docs/contracts/execution-authority-v1.md). Use a registered, enabled runtime and capability; obtain its existing tenant-bound broker client through Pengui's normal vault lifecycle. Do not create a local Chartworks signer or borrow an end-user JWT for the worker.

Merge `examples/chartworks.jobs.json` into the foundation configuration. The new `auth.audiences.jobs` must be an execution-only audience ending in `:execution`, distinct from both ordinary audiences. When using `auth.audiences`, remove the shorthand `auth.audience`. Replace the example issuer URL and tenant with real approved values, and set the environment variables referenced by the broker credentials. The schema contains **references only**. Enable `jobs.enabled` only after the companion endpoint is available.

Configure a retention policy for each tenant before admitting work. A submission uses `{"kind":"retention.sweep","binding_id":"maintenance"}` and one explicit `Idempotency-Key`. The bearer must allow `scheduling.write`, `ops.maintain`, tenant `write`/`erase` and the exact execution-binding `use` reach. Reading and cancelling use `scheduling.read`/`scheduling.cancel` plus addressed `run.read`/`run.write` reach. Schedule creation uses the same admission authority; read/state/manual-run operations additionally use the corresponding signed `schedule` reach. The SDK exposes these typed operations without retaining credentials.

A successful submission reports `pending`, not completed erasure. The worker records its actual terminal outcome. `blocked` means authority or accepted-definition validation failed; after correcting the cause, submit a **new logical key** rather than silently rewriting the accepted job. Cron is five-field/IANA-zone, intervals are anchored, and manual runs share overlap rules. Pause/resume preserves the durable cursor and applies the configured bounded missed-run policy when resumed; pausing does not cancel already accepted jobs.

The same metadata database is the queue; no Redis, message bus or extra scheduler is needed. Multi-replica bounds are pinned on first use. To change the shared concurrency/pending fingerprint, stop all workers, verify there is no active work, and explicitly remove the single `chartworks.queue_limits` configuration row before restarting all replicas with the same new settings. This row contains no authority or secrets.

Turning off `jobs.enabled` stops new admission and dispatch while preserving authorized reads, cancellation and schedule pause. Neither inference nor the broker is required for these retained metadata operations. Back up the metadata database before applying migration003; checksum checks preserve migrations001/002 unchanged. Use `make coverage`, phase 05/06 smoke scripts and `make preflight-full` for real fixture-based validation; later planned phases remain explicit skips in development, not shipped analytics features.

## Vector generations and qualified PostgreSQL sources

The reference metadata image is `pgvector/pgvector:0.8.2-pg17`; install the extension before migrations when using a restricted migration role. Merge the source excerpt from `examples/chartworks.sources.json` into the existing configuration, replace the synthetic tenant/relation coordinates, and provide the read DSN through the referenced environment variable. Never put a resolved credential or human JWT in metadata. The optional write reference is not used by the reader.

The source adapter is qualified for PostgreSQL 17 and the documented ordinary-heap subset. Use explicit source test/discovery and revision-checked rotation; registered metadata is not a health promise. Obtain Pengui-issued action/resource/context reach for the actual operations. See `docs/contracts/vector-sources-validation.md` and the executable source operation inventory. No additional warehouse engine, public raw-SQL route or local model is enabled by this excerpt.
