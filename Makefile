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

.PHONY: help
help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*##/ {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: setup
setup: ## Install required development tools
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	$(GO) install golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)
	$(GO) install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@$(CYCLONEDX_GOMOD_VERSION)

.PHONY: require-tools
require-tools: ## Fail fast if required development tools are missing
	@command -v $(GOLANGCI_LINT) >/dev/null 2>&1 || { printf '%s\n' "golangci-lint not found: run 'make setup'"; exit 1; }
	@command -v $(GOVULNCHECK) >/dev/null 2>&1 || { printf '%s\n' "govulncheck not found: run 'make setup'"; exit 1; }

.PHONY: build
build: ## Build all packages
	$(GO) build ./...

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

.PHONY: download
download: ## Download module dependencies
	$(GO) mod download

.PHONY: vet
vet: ## Run go vet
	$(GO) vet ./...

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
check: require-tools download fmt vet tidy-check lint test-race vulncheck build ## Run all local CI checks (run 'make setup' first)
