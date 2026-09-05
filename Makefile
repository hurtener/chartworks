SHELL := /bin/bash
GO ?= go
GOFILES := $(shell find . -name '*.go' -not -path './_ref/*' -not -path './external_refs/*' -not -path './.git/*' -not -path './vendor/*' 2>/dev/null)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)

.PHONY: build test coverage bench vet lint pg-up pg-down planning-check drift-audit check-mirror preflight preflight-full release-check install-hooks e2e

# The shipping core prefers CGo-free; approved dependency exceptions must update
# the build profile and reference image explicitly. Race tests are a different profile.
build:
	@if [ -z "$(strip $(GOFILES))" ]; then echo "SKIP: build — no Go source files yet"; else \
		CGO_ENABLED=0 $(GO) build ./... && CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o bin/chartworks ./cmd/chartworks; fi

test:
	@if [ -z "$(strip $(GOFILES))" ]; then echo "SKIP: test — no Go source files yet"; else CGO_ENABLED=1 $(GO) test -race ./...; fi

coverage:
	@CGO_ENABLED=1 bash scripts/coverage.sh

bench:
	@if [ -z "$(strip $(GOFILES))" ]; then echo "SKIP: bench — no Go source files yet"; else $(GO) test -run='^$$' -bench=. ./...; fi

vet:
	@if [ -z "$(strip $(GOFILES))" ]; then echo "SKIP: vet — no Go source files yet"; else $(GO) vet ./...; fi

lint:
	@if [ -z "$(strip $(GOFILES))" ]; then echo "SKIP: lint — no Go source files yet"; \
	elif ! command -v golangci-lint >/dev/null 2>&1; then echo "FAIL: golangci-lint is required for implemented code"; exit 1; \
	else golangci-lint run; fi

pg-up:
	@command -v docker >/dev/null 2>&1 || { echo "FAIL: docker is required"; exit 1; }
	@docker compose up -d postgres

pg-down:
	@docker compose down -v

planning-check:
	@python3 -m unittest discover -s scripts -p 'test_*.py'
	@python3 scripts/planning_check.py

drift-audit:
	@bash scripts/drift-audit.sh

check-mirror:
	@diff -q AGENTS.md CLAUDE.md && echo "OK: AGENTS.md and CLAUDE.md are identical"

# Explicit development allowance: a planned phase is reported unimplemented.
# The strict runner itself never silently enables this allowance.
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
