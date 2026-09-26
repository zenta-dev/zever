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

# All Go modules in the repo (143 uses in the committed go.work workspace
# at root), so every *-all target loops per-module with fail-fast `set -e`.
# examples/external-sms is intentionally outside go.work (it proves third-party
# independence); verify it standalone with: GOWORK=off go -C examples/external-sms test ./...
ALL_MODULES := $(shell find . -type f -name go.mod -not -path "./.git/*" -not -path "./examples/external-sms/*" -exec dirname {} \; | sort)

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
sbom: ## Generate a CycloneDX SBOM for the module
	@command -v $(CYCLONEDX_GOMOD) >/dev/null 2>&1 || { printf '%s\n' "cyclonedx-gomod not found: run 'make setup'"; exit 1; }
	$(CYCLONEDX_GOMOD) mod -licenses -std -json -output $(SBOM) .

.PHONY: clean
clean: ## Remove coverage output and build artifacts
	$(GO) clean ./...
	rm -f $(COVERAGE) $(SBOM)

.PHONY: check
check: require-tools download fmt check-all lint-all vulncheck-all ## Run all local CI checks (run 'make setup' first)

# Multi-module targets: per-module loops over the committed go.work workspace.
.PHONY: check-all build-all test-all vet-all tidy-all tidy-check-all lint-all vulncheck-all
check-all: ## Run vet + tidy-check + test (-vet=off -count=1) + build across all modules
	set -e; for d in $(ALL_MODULES); do echo "== $$d =="; (cd $$d && $(GO) vet ./... && $(GO) mod tidy -diff && $(GO) test -vet=off -count=1 ./... && $(GO) build ./...); done

build-all: ## Build all packages in every module
	set -e; for d in $(ALL_MODULES); do echo "== $$d =="; (cd $$d && $(GO) build ./...); done

test-all: ## Run tests in every module
	set -e; for d in $(ALL_MODULES); do echo "== $$d =="; (cd $$d && $(GO) test ./...); done

vet-all: ## Run go vet in every module
	set -e; for d in $(ALL_MODULES); do echo "== $$d =="; (cd $$d && $(GO) vet ./...); done

tidy-all: ## Run go mod tidy in every module
	set -e; for d in $(ALL_MODULES); do echo "== $$d =="; (cd $$d && $(GO) mod tidy); done

tidy-check-all: ## Verify go.mod/go.sum are tidy in every module
	set -e; for d in $(ALL_MODULES); do echo "== $$d =="; (cd $$d && $(GO) mod tidy -diff); done

lint-all: ## Run golangci-lint in every module
	@command -v $(GOLANGCI_LINT) >/dev/null 2>&1 || { printf '%s\n' "golangci-lint not found: run 'make setup'"; exit 1; }
	set -e; for d in $(ALL_MODULES); do echo "== $$d =="; (cd $$d && $(GOLANGCI_LINT) run ./...); done

vulncheck-all: ## Scan every module for known vulnerabilities
	@command -v $(GOVULNCHECK) >/dev/null 2>&1 || { printf '%s\n' "govulncheck not found: run 'make setup'"; exit 1; }
	set -e; for d in $(ALL_MODULES); do echo "== $$d =="; (cd $$d && $(GOVULNCHECK) ./...); done

.PHONY: docs-dev docs-build docs-preview
docs-dev: ## Run docs dev server
	cd docs && npm run dev
docs-build: ## Build docs site
	cd docs && npm run build
docs-preview: ## Preview built docs site
	cd docs && npm run preview
