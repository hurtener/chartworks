SHELL := /bin/bash
GO ?= go
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)

.PHONY: build test coverage bench vet lint pg-up pg-down planning-check drift-audit check-mirror preflight preflight-full release-check install-hooks e2e foundation-smoke

# D-067: the pinned in-process Bruin parser requires its native Rust library.
# Once code exists, missing tooling/source is failure, not planning-only success.
build:
	CGO_ENABLED=1 $(GO) build ./...
	CGO_ENABLED=1 $(GO) build -ldflags "$(LDFLAGS)" -o bin/chartworks ./cmd/chartworks

test:
	CGO_ENABLED=1 $(GO) test -race -count=1 -timeout=20m ./...

coverage:
	@CGO_ENABLED=1 bash scripts/coverage.sh

bench:
	$(GO) test -run='^$$' -bench=. -benchmem ./...

vet:
	$(GO) vet ./...

lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "FAIL: golangci-lint is required"; exit 1; }
	golangci-lint run

pg-up:
	@command -v docker >/dev/null 2>&1 || { echo "FAIL: docker is required"; exit 1; }
	docker compose up -d postgres

pg-down:
	docker compose down -v

planning-check:
	@python3 -m unittest discover -s scripts -p 'test_*.py'
	@python3 scripts/planning_check.py

drift-audit:
	@bash scripts/drift-audit.sh

check-mirror:
	@diff -q AGENTS.md CLAUDE.md && echo "OK: AGENTS.md and CLAUDE.md are identical"

foundation-smoke: build
	@python3 scripts/smoke_foundation.py

# Later planned phases remain explicit unimplemented skips in DEVELOPMENT only.
preflight:
	@CHARTWORKS_ALLOW_PLANNED_SKIP=1 bash scripts/preflight.sh

preflight-full:
	@CHARTWORKS_ALLOW_PLANNED_SKIP=1 PREFLIGHT_FULL=1 bash scripts/preflight.sh

release-check:
	@$(MAKE) planning-check
	@python3 scripts/run_phase_acceptance.py --all --release

install-hooks:
	@bash scripts/install-hooks.sh

e2e:
	CGO_ENABLED=1 $(GO) test -race -p 1 -count=1 ./test/e2e/...
