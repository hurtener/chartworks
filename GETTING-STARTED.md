# Running the phase 01–02 foundation

This build implements configuration, lifecycle/health and the PostgreSQL metadata foundation. It does **not** implement authentication, NLQ, reporting, MCP, rendering or Bifrost inference yet. Those remain separate numbered phases. Pengui is still the sole authority issuer; this build exposes no business endpoint and creates no local credentials.

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

Replace the example host and audience with the real platform configuration. `config-check` performs no network calls or inference. `serve` connects PostgreSQL and applies/checks migrations before listening. The foundation binds **only an explicit loopback IP**; broad listeners are deliberately unavailable until the JWT enforcement phases. Do not publish this foundation through an unauthenticated reverse proxy.

```bash
curl --fail http://127.0.0.1:8080/healthz
curl http://127.0.0.1:8080/readyz
curl --fail http://127.0.0.1:8080/capabilities
```

Liveness is independent of dependency health. Readiness reports `starting`, `ready`, `unavailable` or `stale` for PostgreSQL and trusted verification-key material. The key probe confirms structurally usable public keys, **not** JWT authentication: phase 03 will connect its verifier/cache to this health seam. Failed key refresh never extends key freshness. Optional inference/render capabilities are not enabled or probed.

Use Ctrl+C or SIGTERM to drain the listener, cancel and join dependency monitors, release idle HTTP connections and close the database pool. `chartworks mcp` currently exits 3 with an explicit unavailable message; it does not start a fake MCP server. `/metrics` and all `/v1/*` paths return 404. The real Prometheus exporter is an in-process interface with conformance tests; a protected metrics endpoint is left to the authenticated API phase.

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
make preflight-full
```

Real-store tests create unique `cw_test_*` databases on the explicit test server and remove them afterward. Missing PostgreSQL/client tools, a missing acceptance child, or a skipped runtime test is a failure, not a pass. Coverage instruments production packages across the full test suite, including integration callers; thresholds remain 85% for store, 80% for other internal code and 70% for CLI.

The other 32 phases remain unimplemented and are explicitly reported as planned skips in development preflight. `make release-check` correctly refuses an all-product release while they are planned. This does not block acceptance of phases 01–02.

## Backup, restore and rollback

Run the archive tool as an operator against the **dedicated metadata database**, never as a tenant-facing HTTP action. The DSN is expanded into libpq environment fields, never command-line arguments or logs. Backups are created privately with mode 0600 and atomically published without replacing existing files. Treat the backup itself as sensitive data.

```bash
python3 scripts/store_archive.py backup /secure/directory/chartworks.dump
# Provision a NEW empty database and change CHARTWORKS_STORE_URL to that database.
python3 scripts/store_archive.py restore /secure/directory/chartworks.dump --confirm-empty
```

Restore only trusted archives into an empty database under exclusive operator control: PostgreSQL archives can contain executable SQL. Restore is transactional and rejects nonempty targets; it never uses `--clean`. The `--confirm-empty` check is not a lock against another administrator concurrently modifying the target. URI connection options are explicitly supported/validated by the tool; unrecognized options fail rather than disappear.

The driver verifies ordered migration versions and checksums on startup. A changed, missing or future version is a refusal, not an automatic repair. Recovery means restoring a compatible backup into a new database and deliberately switching configuration; never edit an applied migration or run an invented down migration.

The retention service is the first internal consumer of immutable revisions, compare-and-swap pointers, content-free audit, operation keys and fencing. It is **not** publicly callable before Pengui JWT enforcement. Its scope type is a storage isolation coordinate, not authentication. Expired operation keys retain tombstones so retries cannot silently create fresh work. Old revision references and audit dependencies are preserved; a policy change blocks previously accepted destructive work until a new operation is explicitly accepted.
