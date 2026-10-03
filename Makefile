.DEFAULT_GOAL := help

GO ?= go
GOFMT ?= gofmt
GOLANGCI_LINT ?= golangci-lint
GOVULNCHECK ?= govulncheck
CYCLONEDX_GOMOD ?= cyclonedx-gomod

GOLANGCI_LINT_VERSION ?= v2.13.2
GOVULNCHECK_VERSION ?= v1.8.0
CYCLONEDX_GOMOD_VERSION ?= v1.12.0

COVERAGE ?= coverage.out
SBOM ?= sbom.json

# Every workspace module (144 uses in the committed go.work workspace
# at root), so every *-all target loops per-module with fail-fast `set -e`.
# examples/external-sms and docs/examples stay outside `go.work` by design
# (third-party/consumer proofs); verify them standalone with: GOWORK=off go -C examples/external-sms test ./...
ALL_MODULES := $(shell find . -type f -name go.mod -not -path "./.git/*" -not -path "./.worktrees/*" -not -path "./examples/external-sms/*" -not -path "./docs/examples/*" -exec dirname {} \; | sort)

# Matrix sharding: stable round-robin slice of ALL_MODULES. Defaults
# (TOTAL=1) expand to exactly ALL_MODULES.
SHARD_TOTAL ?= 1
SHARD_INDEX ?= 0
SHARD_MODULES := $(shell printf '%s\n' $(ALL_MODULES) | awk '(NR-1) % $(SHARD_TOTAL) == $(SHARD_INDEX)')

# Explicit module override for scoped CI (e.g. from tools/affected output):
# empty MODULES = current behavior bit-for-bit (all loops use SHARD_MODULES).
MODULES ?=

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*##/ {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: setup
setup: setup-golangci-lint setup-govulncheck setup-cyclonedx-gomod ## Install required development tools

.PHONY: setup-golangci-lint
setup-golangci-lint: ## Install golangci-lint
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: setup-govulncheck
setup-govulncheck: ## Install govulncheck
	$(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)

.PHONY: setup-cyclonedx-gomod
setup-cyclonedx-gomod: ## Install cyclonedx-gomod
	$(GO) install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@$(CYCLONEDX_GOMOD_VERSION)

.PHONY: require-tools
require-tools: ## Fail fast if required development tools are missing
	@command -v $(GOLANGCI_LINT) >/dev/null 2>&1 || { printf '%s\n' "golangci-lint not found: run 'make setup'"; exit 1; }
	@command -v $(GOVULNCHECK) >/dev/null 2>&1 || { printf '%s\n' "govulncheck not found: run 'make setup'"; exit 1; }

.PHONY: build build-release
build: ## Build all packages
	$(GO) build ./...

build-release: ## Build stripped release binary (smaller: -s -w -trimpath)
	$(GO) build -trimpath -ldflags="-s -w" -o bin/zever ./cmd/zever

.PHONY: generate
generate: ## Regenerate editor grammar files from internal/dsl/gengrammar
	$(GO) run ./tools/gengrammar

.PHONY: fmt
fmt: ## Check formatting, fail on unformatted files
	@test -z "$$($(GOFMT) -l .)" || ($(GOFMT) -l . && exit 1)

.PHONY: fmt-fix
fmt-fix: ## Format all files in place
	$(GOFMT) -w .

.PHONY: lint
lint: ## Run golangci-lint (config: .golangci.yml)
	$(GOLANGCI_LINT) run ./...

.PHONY: lint-fix
lint-fix: ## Run golangci-lint with auto-fix
	$(GOLANGCI_LINT) run --fix ./...

.PHONY: test
test: ## Run tests
	$(GO) test ./...

.PHONY: test-race
test-race: ## Run tests with the race detector
	$(GO) test -race ./...

.PHONY: test-lsp
test-lsp: ## Run zever-lsp module tests
	cd tools/zever-lsp && $(GO) test ./...

.PHONY: bench
bench: ## Run benchmarks
	$(GO) test -bench=. -benchmem ./...

.PHONY: cover
cover: ## Run tests with coverage and print the total
	$(GO) test -covermode=atomic -coverprofile=$(COVERAGE) ./...
	$(GO) tool cover -func=$(COVERAGE) | tail -n 1

.PHONY: cover-html
cover-html: cover ## Open the HTML coverage report
	$(GO) tool cover -html=$(COVERAGE)

.PHONY: tidy
tidy: ## Tidy go.mod
	$(GO) mod tidy

.PHONY: tidy-check
tidy-check: ## Verify go.mod and go.sum are tidy
	$(GO) mod tidy -diff

.PHONY: tidy-lsp-check
tidy-lsp-check: ## Verify zever-lsp go.mod/go.sum are tidy
	cd tools/zever-lsp && $(GO) mod tidy -diff

.PHONY: download
download: ## Download module dependencies
	$(GO) mod download

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

.PHONY: vet-lsp
vet-lsp: ## Run go vet on zever-lsp module
	cd tools/zever-lsp && $(GO) vet ./...

.PHONY: vulncheck
vulncheck: ## Scan for known vulnerabilities
	$(GOVULNCHECK) ./...

.PHONY: sbom
sbom: ## Generate a CycloneDX SBOM per module under sbom/ (override list via MODULES)
	@command -v $(CYCLONEDX_GOMOD) >/dev/null 2>&1 || { printf '%s\n' "cyclonedx-gomod not found: run 'make setup'"; exit 1; }
	mkdir -p sbom; set -e; root="$$PWD"; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; slug=$$(echo "$$d" | sed 's|^\./||; s|/|_|g'); (cd $$d && $(CYCLONEDX_GOMOD) mod -licenses -std -json -output "$$root/sbom/$$slug.json" .); done

.PHONY: clean
clean: ## Remove coverage output and build artifacts
	rm -rf sbom .coverage
	rm -f $(COVERAGE) $(SBOM)

.PHONY: check
check: require-tools download fmt check-all modgraph-check lint-all vulncheck-all ## Run all local CI checks (run 'make setup' first)

# Multi-module targets: per-module loops over the committed go.work workspace.
.PHONY: download-all
download-all: ## Download dependencies of every module (override list via MODULES)
	set -e; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do (cd $$d && $(GO) mod download); done

.PHONY: check-all build-all test-all test-race-all test-race-fast vet-all tidy-all tidy-check-all lint-all vulncheck-all
check-all: ## Run vet + tidy-check + test (-vet=off, cached) + build across all modules (shard via SHARD_TOTAL/SHARD_INDEX, override list via MODULES)
	failed=""; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; if ! (cd $$d && $(GO) vet ./... && $(GO) mod tidy -diff && $(GO) test -vet=off ./... && $(GO) build ./...); then failed="$$failed $$d"; echo "::error::module failed: $$d"; fi; done; [ -z "$$failed" ] || { echo "FAILED modules:$$failed"; exit 1; }

test-race-all: ## Run tests with the race detector across all modules (per-module coverage under .coverage/, shard via SHARD_TOTAL/SHARD_INDEX, override list via MODULES)
	mkdir -p .coverage; root="$$PWD"; failed=""; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; slug=$$(echo "$$d" | sed 's|^\./||; s|/|_|g'); if ! (cd $$d && $(GO) test -race -count=1 -covermode=atomic -coverprofile="$$root/.coverage/$$slug.out" ./...); then failed="$$failed $$d"; echo "::error::module failed: $$d"; fi; done; [ -z "$$failed" ] || { echo "FAILED modules:$$failed"; exit 1; }

test-race-fast: ## Run tests with the race detector, no coverage (PR gate; cached, override list via MODULES)
	failed=""; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; if ! (cd $$d && $(GO) test -race -vet=off ./...); then failed="$$failed $$d"; echo "::error::module failed: $$d"; fi; done; [ -z "$$failed" ] || { echo "FAILED modules:$$failed"; exit 1; }

.PHONY: modgraph-check
modgraph-check: ## Verify every module requires+replaces the intra-repo modules it imports (CI module-graph gate)
	$(GO) run ./tools/modgraph --check

.PHONY: test-flake
test-flake: ## Hunt flaky tests: race + shuffle + repeat across modules (FLAKE_COUNT=5, override list via MODULES)
	failed=""; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; if ! (cd $$d && $(GO) test -race -vet=off -count=$(or $(FLAKE_COUNT),5) -shuffle=on ./...); then failed="$$failed $$d"; echo "::error::flaky/failing module: $$d"; fi; done; [ -z "$$failed" ] || { echo "FAILED modules:$$failed"; exit 1; }

build-all: ## Build all packages in every module (override list via MODULES)
	set -e; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; (cd $$d && $(GO) build ./...); done

test-all: ## Run tests in every module (override list via MODULES)
	set -e; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; (cd $$d && $(GO) test ./...); done

vet-all: ## Run go vet in every module (override list via MODULES)
	set -e; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; (cd $$d && $(GO) vet ./...); done

tidy-all: ## Run go mod tidy in every module (override list via MODULES)
	set -e; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; (cd $$d && $(GO) mod tidy); done

tidy-check-all: ## Verify go.mod/go.sum are tidy in every module (override list via MODULES)
	set -e; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; (cd $$d && $(GO) mod tidy -diff); done

deps-sync: ## Reconcile dependency versions across all modules after a bump
	$(MAKE) tidy-all
	GOWORK=off go -C docs/examples mod tidy
	GOWORK=off go -C examples/external-sms mod tidy

deps-sync-check: ## Verify no dependency drift remains after a bump
	$(MAKE) tidy-check-all
	GOWORK=off go -C docs/examples mod tidy -diff
	GOWORK=off go -C examples/external-sms mod tidy -diff

lint-all: ## Run golangci-lint in every module (shard via SHARD_TOTAL/SHARD_INDEX, override list via MODULES)
	@command -v $(GOLANGCI_LINT) >/dev/null 2>&1 || { printf '%s\n' "golangci-lint not found: run 'make setup'"; exit 1; }
	failed=""; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; if ! (cd $$d && $(GOLANGCI_LINT) run ./...); then failed="$$failed $$d"; echo "::error::module failed: $$d"; fi; done; [ -z "$$failed" ] || { echo "FAILED modules:$$failed"; exit 1; }

vulncheck-all: ## Scan every module for known vulnerabilities (override list via MODULES)
	@command -v $(GOVULNCHECK) >/dev/null 2>&1 || { printf '%s\n' "govulncheck not found: run 'make setup'"; exit 1; }
	failed=""; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; if ! (cd $$d && $(GOVULNCHECK) ./...); then failed="$$failed $$d"; echo "::error::module failed: $$d"; fi; done; [ -z "$$failed" ] || { echo "FAILED modules:$$failed"; exit 1; }

.PHONY: docs-dev docs-build docs-preview
docs-dev: ## Run docs dev server
	cd docs && npm run dev
docs-build: ## Build docs site
	cd docs && npm run build
docs-preview: ## Preview built docs site
	cd docs && npm run preview
