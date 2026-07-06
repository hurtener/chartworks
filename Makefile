SHELL := /bin/bash

GO ?= go

# Every Go source file, excluding the predecessor reference copies (_ref/,
# git-ignored), the mined external references (external_refs/, not part of this
# module), and VCS metadata. Used to no-op build/test/vet/lint gracefully before
# any Go code exists.
GOFILES := $(shell find . -name '*.go' \
	-not -path './_ref/*' \
	-not -path './external_refs/*' \
	-not -path './.git/*' \
	-not -path './vendor/*' 2>/dev/null)

# Version injection: computed from git, falling back to dev/none/unknown when
# git is unavailable so `make build` never hard-fails on version metadata.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE := $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)

.PHONY: build test coverage bench vet lint pg-up pg-down drift-audit check-mirror preflight preflight-full install-hooks e2e

## build: go build ./... (full compile check) + the chartworks binary with
## version ldflags to bin/chartworks. CGo-free posture (CGO_ENABLED=0) — unlike
## Soundings, Chartworks has no seam that requires CGo. SKIPs when no Go source
## exists.
build:
	@if [ -z "$(strip $(GOFILES))" ]; then \
		echo "SKIP: build — no Go source files yet"; \
	else \
		CGO_ENABLED=0 $(GO) build ./... && \
		CGO_ENABLED=0 $(GO) build -ldflags "$(LDFLAGS)" -o bin/chartworks ./cmd/chartworks; \
	fi

## test: go test -race ./... — SKIPs when no Go source exists yet.
test:
	@if [ -z "$(strip $(GOFILES))" ]; then \
		echo "SKIP: test — no Go source files yet"; \
	else \
		$(GO) test -race ./...; \
	fi

## coverage: the mechanical coverage-band gate (CLAUDE.md §11).
coverage:
	@bash scripts/coverage.sh

## bench: go benchmarks — a baseline, not a CI gate. SKIPs when no Go source.
bench:
	@if [ -z "$(strip $(GOFILES))" ]; then \
		echo "SKIP: bench — no Go source files yet"; \
	else \
		$(GO) test -run='^$$' -bench=. ./...; \
	fi

## vet: go vet ./... — SKIPs when no Go source exists yet.
vet:
	@if [ -z "$(strip $(GOFILES))" ]; then \
		echo "SKIP: vet — no Go source files yet"; \
	else \
		$(GO) vet ./...; \
	fi

## lint: golangci-lint run — SKIPs when no Go source, or golangci-lint absent.
lint:
	@if [ -z "$(strip $(GOFILES))" ]; then \
		echo "SKIP: lint — no Go source files yet"; \
	elif ! command -v golangci-lint >/dev/null 2>&1; then \
		echo "SKIP: lint — golangci-lint not installed"; \
	else \
		golangci-lint run; \
	fi

## pg-up: start the local Postgres (docker-compose.yml, port 5434) — SKIPs when
## docker absent.
pg-up:
	@if ! command -v docker >/dev/null 2>&1; then \
		echo "SKIP: pg-up — docker not installed"; \
	else \
		docker compose up -d postgres; \
	fi

## pg-down: stop and remove the local Postgres.
pg-down:
	@if ! command -v docker >/dev/null 2>&1; then \
		echo "SKIP: pg-down — docker not installed"; \
	else \
		docker compose down -v; \
	fi

## drift-audit: mechanical design-coherence checks (CLAUDE.md §16).
drift-audit:
	@bash scripts/drift-audit.sh

## check-mirror: AGENTS.md and CLAUDE.md must be byte-identical (CLAUDE.md §18).
check-mirror:
	@diff -q AGENTS.md CLAUDE.md && echo "OK: AGENTS.md and CLAUDE.md are identical"

## preflight: build + smoke checks + drift-audit (CLAUDE.md §4.1 — non-negotiable gate).
## Fast by default (only phases whose owning packages changed vs HEAD); the
## pre-commit hook uses this. CI / wave-end use `make preflight-full`.
preflight:
	@bash scripts/preflight.sh

## preflight-full: preflight over EVERY phase's smoke (CI / wave-end; §14 checklist).
preflight-full:
	@PREFLIGHT_FULL=1 bash scripts/preflight.sh

## install-hooks: install the pre-commit hook (one-time, per clone).
install-hooks:
	@bash scripts/install-hooks.sh

## e2e: the full end-to-end release proof (once authored). SKIPs without a store
## URL, so run `make pg-up` and export CHARTWORKS_TEST_STORE_URL first.
e2e:
	$(GO) test -race -p 1 -count=1 ./test/e2e/...
