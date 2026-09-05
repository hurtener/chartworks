# Running the phase 01–04 verified foundation

This build implements configuration, lifecycle/health, PostgreSQL metadata, Pengui JWT verification and signed-scope enforcement on its operational APIs. It does **not** implement NLQ, reporting, the full MCP server, rendering or Bifrost inference yet. Pengui remains the sole authority issuer; Chartworks creates no credentials or local identity policy.

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

Liveness is independent of dependency health. Readiness reports `starting`, `ready`, `unavailable` or `stale` for PostgreSQL and trusted verification-key material. The real JWT verifier and readiness use the same bounded trusted public-key cache; a readiness result does not authorize an individual request. Failed key refresh never extends key freshness. Optional inference/render capabilities are not enabled or probed.

Use Ctrl+C or SIGTERM to drain the listener, cancel and join dependency monitors, release idle HTTP connections and close the database pool. `chartworks mcp` currently exits 3 with an explicit unavailable message; it does not start a fake MCP server. Operational `/v1/*` and `/metrics` requests now require a valid Pengui bearer plus their separately registered action and addressed reach. Anonymous requests return 401; a valid caller receives a nondisclosing 404 for unregistered/inaccessible resources. Metrics also needs `ops.metrics` and returns 404 when disabled. Use the operation manifest below; there is no local login or default administrator token.

## Configuration contract

JSON is the implemented format. `config-check --defaults` prints the exact typed defaults, with secret references rather than resolved credentials. Precedence is defaults -> file -> explicitly named `env:NAME` references -> explicit `--listen` override. `--config` selects the file before `CHARTWORKS_CONFIG`; arbitrary environment variables do not silently override fields. See [the complete key reference](docs/configuration.md).

The existing `examples/chartworks.gateway.json` is a future gateway excerpt. The foundation accepts and validates its remote-provider shapes when combined with required foundation configuration, but cannot enable inference. No SDK call, local model download or provider-secret resolution happens in this phase. Phase 05 owns Bifrost initialization and the remote embedding/rerank/completion consumers.

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
