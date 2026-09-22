# Opt-in live commerce gate

`TestCommerceSyntheticFixture` and `TestCommerceReportingRecorded` exercise a disposable PostgreSQL 17 database, the native validator, reviewed topic publication and retained report/render paths without paid inference. They require `CHARTWORKS_TEST_STORE_URL` and the pinned Bruin native parser library. The PostgreSQL URL must have an explicit user, database and `sslmode=disable` on loopback, or use the repository's configured TLS form.

`TestLiveCommerceGatewayE2E` is a separate paid operator gate. Run it only with an isolated disposable PostgreSQL 17 instance, the merged Bifrost OpenRouter custom rerank provider, a built `chartworks-renderer`, and a fresh artifact directory outside the checkout. It reads the OpenRouter key only from `CHARTWORKS_OPENROUTER_API_KEY`. The key and both database DSNs stay in the operator environment. No provider call occurs on construction or in ordinary tests.

```sh
export CHARTWORKS_TEST_STORE_URL='postgres://<operator>@127.0.0.1:<port>/<disposable-db>?sslmode=disable'
export CHARTWORKS_OPENROUTER_API_KEY='<operator-secret>'
export CGO_LDFLAGS='-L<absolute-private-bruin-rustffi-release-directory>'
export CHARTWORKS_LIVE_RENDERER='<absolute-path-to-built-chartworks-renderer>'
export CHARTWORKS_LIVE_ARTIFACT_DIR='<fresh-absolute-directory-outside-git>'
CHARTWORKS_LIVE_E2E=1 go test ./test/acceptance -run '^TestLiveCommerceGatewayE2E$' -count=1 -timeout=8m -v
```

Build the renderer from the same tested commit with `go build -o "$CHARTWORKS_LIVE_RENDERER" ./cmd/chartworks-renderer`. The native parser build is documented in `scripts/build-bruin.sh`; do not use a different parser commit for the gate.

The fixed seed in `test/acceptance/testdata/live_commerce.sql` has four customers, six orders across January–March 2026, eight items and four refunds. Paid-order gross is **640.00 USD**, refunds against paid orders are **125.00 USD**, and net is **515.00 USD**. Directly joining raw items and refunds inflates totals, so the live net question asserts the correct grain. The reviewed commerce topic names this rule and includes a separate customer-retention distractor; the test sends both candidates through the Bifrost custom rerank provider and requires scored results. It publishes both topics through the ordinary review lifecycle, then asks English, Spanish, exact net and underspecified questions through Plan→Run. A wrong tenant, wrong execution context and missing signed source reach must fail.

The gate creates a reviewed block and a published report, then uses the real retained delivery viewer and supervised renderer to write `report.html`, `chart.svg`, `viewer-table.json`, `viewer-chart.json`, `embedding-receipt.json`, `rerank-receipt.json`, `report-receipt.json` and `receipts.json`. JSON receipts contain status, IDs, counts and bounded model usage only; they omit raw prompts, generated SQL, credentials and provider response bodies. HTML/SVG and viewer projections contain only synthetic warehouse values. Save Chrome screenshots of the generated HTML/SVG beside these artifacts after the run; screenshots are a separate visual check. A green recorded test does not establish live provider quality, and this opt-in gate does not mark Phase 25 released.
