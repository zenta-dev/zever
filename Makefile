.DEFAULT_GOAL := help

GO ?= go
GOFMT ?= gofmt
GOLANGCI_LINT ?= golangci-lint
GOVULNCHECK ?= govulncheck
CYCLONEDX_GOMOD ?= cyclonedx-gomod

GOLANGCI_LINT_VERSION ?= v2.14.0
GOVULNCHECK_VERSION ?= v1.8.0
CYCLONEDX_GOMOD_VERSION ?= v1.12.0

COVERAGE ?= coverage.out
SBOM ?= sbom.json

# Every workspace module (174 uses in the committed go.work workspace
# at root), so every *-all target loops per-module with fail-fast `set -e`.
# examples/external-sms and docs/examples stay outside `go.work` by design
# (third-party/consumer proofs); verify them standalone with: GOWORK=off go -C examples/external-sms test ./...
# The harness checkout dirs (.git, .worktrees, .claude) are skipped at ANY
# depth, like tools/modgraph and tools/tag-release.sh do: a worktree nested
# under a module dir holds a full checkout of the repo, so counting it made
# the module list differ from the tag list. Without the exclusion the list
# balloons past ARG_MAX, `printf` fails, and every *-all loop silently tests
# nothing (false green).
# Go's own tooling ignores testdata trees; compile-fail fixture modules
# (container/testdata/compilefail/*) must fail `go build` by design, so
# they are excluded here like the harness checkouts above.
ALL_MODULES := $(shell find . -type f -name go.mod -not -path "*/.git/*" -not -path "*/.worktrees/*" -not -path "*/.claude/*" -not -path "./examples/external-sms/*" -not -path "./docs/examples/*" -not -path "*/testdata/*" -exec dirname {} \; | sort)

# The two modules that stay outside `go.work` by design (third-party and
# consumer proofs). They are not workspace members, so the go command
# refuses to build them with the workspace loaded: every go invocation
# below runs with GOWORK=off. They are tidied with the same offline
# machinery as the workspace modules (tools/gotidy.sh), so a sibling that
# has no proxy release yet resolves from disk instead of the proxy.
OUTSIDE_MODULES := docs/examples examples/external-sms

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
	$(GO) build -trimpath -ldflags="-s -w -X main.cliVersion=$$(tr -d '[:space:]' < cmd/zever/VERSION)" -o bin/zever ./cmd/zever

.PHONY: generate
generate: ## Regenerate editor grammar files from dsl/gengrammar
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

.PHONY: err-lint
err-lint: ## Scan for error-convention violations (unprefixed messages, bracket prefixes, double/missing %w, stray panics)
	$(GO) -C tools/errscan run . "$(CURDIR)"

.PHONY: check-tags
check-tags: ## Scan for struct-tag wire-format violations (empty tags, yaml/toml outside the allowlist, v0.7.2 wire structs losing json tags or wire keys)
	$(GO) -C tools/tagcheck run . "$(CURDIR)"

.PHONY: tag-golden
tag-golden: ## Regenerate the pinned json wire keys of the struct-tag guard (needs the v0.7.2 tag and the full history)
	$(GO) -C tools/tagcheck run ./cmd/taggolden -root "$(CURDIR)"

.PHONY: tag-golden-check
tag-golden-check: ## Verify the pinned json wire keys still match the git history (needs the v0.7.2 tag and the full history)
	$(GO) -C tools/tagcheck run ./cmd/taggolden -root "$(CURDIR)" -check

.PHONY: apidiff
apidiff: ## Check Core-tier public API against last release tags (fails on undocumented breaking changes)
	$(GO) run ./tools/apidiff

.PHONY: test
test: ## Run tests
	$(GO) test ./...

.PHONY: installer-test
installer-test: ## Run installer fail-closed tests (offline, stubbed PATH)
	sh tools/installer-test.sh

.PHONY: version-check
version-check: ## Fail if cmd/zever/VERSION is lower than the newest cmd/zever/v* tag (skips without tags)
	bash tools/check-version.sh

.PHONY: changelog-lint
changelog-lint: ## Lint CHANGELOG.md structure (unique headings, **Breaking:** prefix, indentation)
	bash tools/changelog-lint.sh

.PHONY: changelog-compile
changelog-compile: ## Compile .changelog/unreleased/*.md fragments into the changelog's pending section
	bash tools/changelog-compile.sh

.PHONY: changelog-compile-check
changelog-compile-check: ## Assert the changelog keeps a pending ## [Unreleased] section (the release-cut invariant)
	bash tools/changelog-compile.sh --check

.PHONY: changelog-compile-test
changelog-compile-test: ## Test the changelog fragment compiler (pending-section rule)
	bash tools/changelog-compile-test.sh

.PHONY: sbom-test
sbom-test: ## Test the SBOM resolution script (offline: empty proxy, stubbed cyclonedx-gomod)
	bash tools/sbom-test.sh

.PHONY: compile-check
compile-check: ## Verify committed generated code matches the schema (zever compile --check over every example)
	cd examples/todo && $(GO) run ../../cmd/zever compile --check --out ./generated schema/todo.zen schema/todo_grpc.zen
	cd examples/bookings && $(GO) run ../../cmd/zever compile --check --backend=atlas,gogen,openapi,proto,protogogen,zenorm --out ./generated schema/bookings.zen
	cd examples/demoapp && $(GO) run ../../cmd/zever compile --check --backend=atlas,gogen,openapi,proto,protogogen,zenorm --out ./generated schema/app.zen
	cd examples/showcase && $(GO) run ../../cmd/zever compile --check --backend=atlas,gogen,openapi,proto,protogogen,zenorm --out ./generated schema/shop/shop.zen
	$(GO) run ./cmd/zever compile --check --backend=zenorm --out examples/json_query/generated examples/json_query/schema/json_query.zen
	$(GO) run ./cmd/zever compile --check --backend=zenorm --out examples/bulk_upsert/generated examples/bulk_upsert/schema/bulk_upsert.zen

.PHONY: compilefail-check
compilefail-check: ## Assert typed-Ref misuse fixtures fail to build (stream G gate)
	bash tools/ci-compilefail.sh

.PHONY: fuzz-smoke-config
fuzz-smoke-config: ## 60s fuzz smoke for config .env parser targets (override: FUZZTIME=10s, stream G gate)
	bash tools/ci-env-fuzz-smoke.sh

.PHONY: quickstart-check
quickstart-check: ## Headless 3-command journey: new -> dev boot -> readiness probe (stream G gate)
	TMPD="$$(mktemp -d)" && trap 'rm -rf "$$TMPD"' EXIT INT TERM && \
	go build -o "$$TMPD/zever" ./cmd/zever && \
	ZEVER_FRAMEWORK_PATH="$(CURDIR)" bash tools/ci-quickstart-dev.sh "$$TMPD/zever"

.PHONY: test-race
test-race: ## Run tests with the race detector
	$(GO) test -race ./...

.PHONY: test-lsp
test-lsp: ## Run zever-lsp module tests
	cd tools/zever-lsp && $(GO) test ./...

.PHONY: bench
bench: ## Run benchmarks in every module (shard via SHARD_TOTAL/SHARD_INDEX, override list via MODULES)
	failed=""; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; if ! (cd $$d && $(GO) test -run='^$$' -bench=. -benchmem ./...); then failed="$$failed $$d"; echo "::error::module failed: $$d"; fi; done; [ -z "$$failed" ] || { echo "FAILED modules:$$failed"; exit 1; }

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
tidy-check: ## Verify go.mod/go.sum are tidy (no root module; delegates to the per-module target)
	$(MAKE) tidy-check-all

.PHONY: tidy-lsp-check
tidy-lsp-check: ## Verify zever-lsp go.mod/go.sum are tidy
	cd tools/zever-lsp && $(CURDIR)/tools/gotidy.sh -diff

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

# Own modules live in subdirs without a LICENSE file, so cyclonedx-gomod cannot detect them; stamp the root LICENSE.
SBOM_OWN_PREFIX ?= github.com/zenta-dev/zever
SBOM_OWN_LICENSE ?= Apache-2.0

.PHONY: sbom
sbom: ## Generate a CycloneDX SBOM per module under sbom/ (override list via MODULES)
	@command -v $(CYCLONEDX_GOMOD) >/dev/null 2>&1 || { printf '%s\n' "cyclonedx-gomod not found: run 'make setup'"; exit 1; }
	@command -v jq >/dev/null 2>&1 || { printf '%s\n' "jq not found: install jq to stamp own-module licenses"; exit 1; }
	mkdir -p sbom; set -e; root="$$PWD"; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; slug=$$(echo "$$d" | sed 's|^\./||; s|/|_|g'); GO="$(GO)" CYCLONEDX_GOMOD="$(CYCLONEDX_GOMOD)" SBOM_OWN_PREFIX="$(SBOM_OWN_PREFIX)" SBOM_OWN_LICENSE="$(SBOM_OWN_LICENSE)" SBOM_REPO_ROOT="$$root" $(CURDIR)/tools/sbom.sh "$$d" "$$root/sbom/$$slug.json"; done

.PHONY: clean
clean: ## Remove coverage output and build artifacts
	rm -rf sbom .coverage
	rm -f $(COVERAGE) $(SBOM)

.PHONY: check
check: require-tools download fmt check-all check-tags err-lint modgraph-check dependabot-check lint-all vulncheck-all installer-test changelog-compile-test changelog-compile-check sbom-test compile-check version-check changelog-lint ## Run all local CI checks (run 'make setup' first)

# Multi-module targets: per-module loops over the committed go.work workspace.
.PHONY: download-all
download-all: ## Download dependencies of every module (override list via MODULES)
	set -e; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do (cd $$d && $(GO) mod download); done

.PHONY: check-all build-all test-all test-race-all test-race-fast test-race-pr vet-all tidy-all tidy-check-all lint-all vulncheck-all
check-all: ## Run vet + tidy-check + test (-vet=off, cached) + build across all modules (shard via SHARD_TOTAL/SHARD_INDEX, override list via MODULES)
	failed=""; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; if ! (cd $$d && $(GO) vet ./... && $(CURDIR)/tools/gotidy.sh -diff && $(GO) test -vet=off ./... && $(GO) build ./...); then failed="$$failed $$d"; echo "::error::module failed: $$d"; fi; done; [ -z "$$failed" ] || { echo "FAILED modules:$$failed"; exit 1; }
	failed=""; for d in $(OUTSIDE_MODULES); do echo "== $$d =="; if ! (cd $$d && GOWORK=off $(CURDIR)/tools/gotidy.sh -diff); then failed="$$failed $$d"; echo "::error::module failed: $$d"; fi; done; [ -z "$$failed" ] || { echo "FAILED modules:$$failed"; exit 1; }

test-race-all: ## Run tests with the race detector across all modules (per-module coverage under .coverage/, shard via SHARD_TOTAL/SHARD_INDEX, override list via MODULES)
	mkdir -p .coverage; root="$$PWD"; failed=""; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; slug=$$(echo "$$d" | sed 's|^\./||; s|/|_|g'); if ! (cd $$d && $(GO) test -race -count=1 -covermode=atomic -coverprofile="$$root/.coverage/$$slug.out" ./...); then failed="$$failed $$d"; echo "::error::module failed: $$d"; fi; done; [ -z "$$failed" ] || { echo "FAILED modules:$$failed"; exit 1; }

test-race-fast: ## Run tests with the race detector, no coverage (PR gate; cached, override list via MODULES)
	failed=""; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; if ! (cd $$d && $(GO) test -race -vet=off ./...); then failed="$$failed $$d"; echo "::error::module failed: $$d"; fi; done; [ -z "$$failed" ] || { echo "FAILED modules:$$failed"; exit 1; }

test-race-pr: ## Run tests with the race detector, shuffled order, 2 iterations (PR order-dependence gate; seed via SHUFFLE_SEED or random, override list via MODULES)
	failed=""; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; if ! (cd $$d && $(GO) test -race -vet=off -count=2 -shuffle=$(or $(SHUFFLE_SEED),on) ./...); then failed="$$failed $$d"; echo "::error::module failed: $$d"; fi; done; [ -z "$$failed" ] || { echo "FAILED modules:$$failed"; exit 1; }

.PHONY: modgraph-check
modgraph-check: ## Verify every module requires+replaces the intra-repo modules it imports, and every go.mod follows the release tag path rule (CI module-graph gate)
	$(GO) run ./tools/modgraph --check
	$(GO) run ./tools/modgraph --check-pins

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
	set -e; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; (cd $$d && $(CURDIR)/tools/gotidy.sh); done
	set -e; for d in $(OUTSIDE_MODULES); do echo "== $$d =="; (cd $$d && GOWORK=off $(CURDIR)/tools/gotidy.sh); done

tidy-check-all: ## Verify go.mod/go.sum are tidy in every module (override list via MODULES)
	set -e; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; (cd $$d && $(CURDIR)/tools/gotidy.sh -diff); done
	set -e; for d in $(OUTSIDE_MODULES); do echo "== $$d =="; (cd $$d && GOWORK=off $(CURDIR)/tools/gotidy.sh -diff); done

deps-sync: ## Reconcile dependency versions across all modules after a bump
	$(MAKE) tidy-all

.PHONY: release-prep
release-prep: ## Pin the repo for a release: VERSION=vX.Y.Z rewrites cmd/zever/VERSION, installers, docs pins and every intra-repo go.mod require, then tidies
	@test -n "$(VERSION)" || { echo "usage: make release-prep VERSION=vX.Y.Z" >&2; exit 1; }
	bash tools/release-prep.sh $(VERSION)

deps-sync-check: ## Verify no dependency drift remains after a bump
	$(MAKE) tidy-check-all

.PHONY: dependabot-check
dependabot-check: ## Verify .github/dependabot.yml lists every module (regenerate: bash tools/gen-dependabot.sh)
	bash tools/gen-dependabot.sh --check

lint-all: ## Run golangci-lint in every module (shard via SHARD_TOTAL/SHARD_INDEX, override list via MODULES)
	@command -v $(GOLANGCI_LINT) >/dev/null 2>&1 || { printf '%s\n' "golangci-lint not found: run 'make setup'"; exit 1; }
	failed=""; for d in $(if $(MODULES),$(MODULES),$(SHARD_MODULES)); do echo "== $$d =="; if ! (cd $$d && $(GOLANGCI_LINT) run ./...); then failed="$$failed $$d"; echo "::error::module failed: $$d"; fi; done; [ -z "$$failed" ] || { echo "FAILED modules:$$failed"; exit 1; }

vulncheck-all: ## Scan every module; reachable vulns fail unless allowlisted with an unexpired entry (override list via MODULES)
	@command -v $(GOVULNCHECK) >/dev/null 2>&1 || { printf '%s\n' "govulncheck not found: run 'make setup'"; exit 1; }
	$(GO) run ./tools/vulngate --govulncheck "$(GOVULNCHECK)" $(if $(MODULES),$(MODULES),$(SHARD_MODULES))

.PHONY: docs-dev docs-build docs-preview docs-audit docs-lint docs-links
docs-dev: ## Run docs dev server
	cd docs && npm run dev
docs-build: ## Build docs site
	cd docs && npm run build
docs-preview: ## Preview built docs site
	cd docs && npm run preview

# Docs gates: they cross-check the published pages against the Go source.
# They need the docs dependencies once (npm ci in docs/).
.PHONY: docs-audit
docs-audit: ## Audit docs claims against the Go code (env keys, adapters, flags, imports)
	cd docs && node scripts/audit-facts.mjs

.PHONY: docs-audit-self-test
docs-audit-self-test: ## Prove the docs audit rejects the env-key mistakes it guards against
	cd docs && node scripts/audit-facts.mjs --self-test

.PHONY: docs-lint
docs-lint: ## Lint docs style (em/en dashes, sentence case, Asides)
	cd docs && node scripts/lint-docs.mjs

.PHONY: docs-links
docs-links: ## Check every internal link in the built site (run make docs-build first)
	cd docs && node scripts/check-links.mjs
