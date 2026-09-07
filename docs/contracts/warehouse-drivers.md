# Warehouse read drivers — phase 14 support boundary

The shared source registry accepts six explicit dialects: PostgreSQL, MySQL,
SQL Server, BigQuery, Snowflake and Databricks. Empty dialect retains PostgreSQL
compatibility. Every engine uses the same validator-issued plan, signed source and
context reach, source revision fence, typed result collector and operation journal.
An operator connection alias is not caller-selected connection information.

The implementation dependency is the selective Bruin fork at
`1fad0ffbf27b9ec1349c6a9859fb3e5d55107333` (upstream v0.11.749 ancestry).
Chartworks imports leaf packages and the native parser, not the universal manager.
The supervised managed-write runner remains separate from read execution.

| Engine | Executed qualification boundary | Parameters and values | Cancellation boundary |
| --- | --- | --- | --- |
| PostgreSQL 17 | Existing real PostgreSQL/pgvector source and execution suites | Existing exact native scalar contract | Original owned connection and durable native attempt evidence |
| MySQL 8.4 | Real container suite required in CI | Native bound scalars, exact decimal text/results, binary/null; closed supported types | Owned connection plus exact native identity reconciliation; no blind connection-ID reuse |
| SQL Server 2022 | Native Linux amd64 Developer container suite required in CI; Mac emulation is not qualification | Native integer/text/bool/null binding; exact integer/decimal/binary/temporal results. Decimal input is explicitly unsupported by the current driver binding seam | Local owned connection cancellation and acknowledged cleanup. Restart cancellation is unsupported; exact DMV observations can remain indeterminate |
| BigQuery | Recorded SDK/HTTP and source lifecycle fixtures only | Closed scalar binding; exact decimal/integer/time/binary/null. Arrays/records unsupported | Deterministic project/location/job identity; native cancel plus bounded status reconciliation |
| Snowflake | Recorded protocol and source lifecycle fixtures only | Closed scalar/result support, with unsupported types rejected | Request identity is distinct from acknowledged query identity; lost replies remain uncertain |
| Databricks | Recorded Statement Execution API and source lifecycle fixtures only | Closed scalar binding; binary result codec unsupported | Workspace/warehouse/attempt precedes actual statement ID; lost submit reply is uncertain and never automatically resubmitted |

Cloud rows in this matrix describe implementation scope, not live deployment
qualification. No live cloud credentials were requested or used. Cloud migration
cutover requires separately approved evidence; recorded fixtures do not establish
cloud latency, account policy, availability or transport behavior in production.

SQL Server source discovery currently supports ordinary base tables on versions
16/17, with exact native column metadata. Views, temporal/memory-optimized/file
tables, computed/generated/encrypted/custom/CLR columns and enabled row policies
are explicitly outside this subset. Native table metadata is rechecked under a
retained table lock. The configured reader must have SELECT plus metadata visibility
on declared schemas and SHOWPLAN, without effective write/control permissions.
SHOWPLAN supplies native planning evidence for the admitted subset, not a universal
function/dependency safety theorem. Native server name is not a boot epoch.

Result row and byte caps count the serialized schema and rows in the shared
collector. They do not assert a bounded wire allocation inside every SDK, nor a
warehouse scan-byte guarantee. Driver page size hints are not server cost ceilings.
Secret rotation closes old leaf clients; retained metadata reads stay independent
of connector availability.

## Build and fixture reproduction

`bash scripts/build-bruin.sh` requires an absolute private
`CHARTWORKS_BRUIN_BUILD_DIR`. It fetches the immutable fork, builds the locked native
parser with Rust 1.98.1 and the SQL runner with Go 1.26.4. Set `CGO_LDFLAGS` to
`-L<build-dir>/source/pkg/sqlparser/rustffi/target/release`, then run `make build`.
The script exports that path through `GITHUB_ENV` in CI. Libraries remain outside
both the checkout and Go module cache. Chartworks shipping builds now require CGo;
CI compiles and executes Linux amd64 and macOS arm64 binaries on their native
platforms, rather than pretending that a CGo-disabled cross-build is functional.

CI uses disposable PostgreSQL17, MySQL8.4 and SQL Server2022-CU23 containers. The
SQL Server Developer edition is for development/testing, not a production license;
its container configuration explicitly accepts the vendor EULA. See Microsoft's
[container quickstart](https://learn.microsoft.com/en-us/sql/linux/quickstart-install-connect-docker?view=sql-server-ver17).
Fixtures generate random namespaces and SELECT-only synthetic accounts and clean
them up. Administrator fixture DSNs are provided only via
`CHARTWORKS_TEST_STORE_URL`, `CHARTWORKS_TEST_MYSQL_DSN` and
`CHARTWORKS_TEST_SQLSERVER_DSN`. Missing native fixtures fail `TestPhase14`; they do
not produce passing skips.

`TestPhase14/AC01`–`AC06` exercise actual source creation/discovery, native validation,
read-only accounts, exact values, caps, cancellation, rotation and repeatable seeds.
Mandatory leaf protocol suites and `TestBigQuerySourceLifecycleRecorded`,
`TestSnowflakeSourceLifecycleRecorded` and `TestDatabricksSourceLifecycleRecorded`
supplement the named criteria for cloud engines. The latter exercise the actual
Chartworks Service using private per-instance recorded client factories; they do
not replace the fork tests against real SDK/HTTP protocol fixtures. Hosted CI must pass on the final committed head before
these pending implementations are described as qualified.

The root `Dockerfile` is the reference deployment image. Its Go1.26.4/Rust1.98.1
build and Debian bookworm runtime share the same libc ABI; the Rust parser is
statically linked into the CGo binary, while runtime libc and C++ dependencies are
installed explicitly. The image runs as UID/GID10001 and contains the pinned
managed SQL runner at `/usr/local/libexec/chartworks-bruin`. Build with
`docker build -t chartworks-reference .`; the image executes a version smoke during
build, and CI executes it again without network and with a read-only root filesystem.
Actual serving still requires operator configuration and supplied Pengui authority.
Pipeline execution additionally requires an executable private tmpfs (for example,
`--tmpfs /run/chartworks-pipelines:rw,exec,nosuid,nodev,size=2g,uid=10001,gid=10001,mode=0700`)
and the matching configured pipeline directory. No warehouse credentials or generated
pipeline workspaces are baked into the image.
