# This file is updated automatically by hack/do.sh. Do not edit it directly.
.DEFAULT_GOAL := help

##@ Main Build Targets
.PHONY: build
build: lint test ## Build the main binary after running linters and tests
	@hack/do.sh build

.PHONY: build-all
build-all: ## Force build of all packages and dependencies
	@hack/do.sh build -a

.PHONY: install
install: ## Compile and install the main binary to $GOPATH/bin or $GOBIN
	@hack/do.sh install

##@ Development & Verification
.PHONY: fmt
fmt: ## Format Go source code using go fmt
	@hack/do.sh fmt

.PHONY: lint
lint: ## Run default golangci-lint linters
	@hack/do.sh lint

.PHONY: lint-more
lint-more: ## Run golangci-lint with additional linters enabled
	@hack/do.sh lint_more

.PHONY: vet
vet: ## Run Go vet static analysis checks
	@hack/do.sh vet

.PHONY: test
test: ## Run unit tests with coverage reporting
	@hack/do.sh test

.PHONY: race
race: ## Run unit tests with the race condition detector enabled
	@hack/do.sh race

.PHONY: bench
bench: ## Run benchmarks
	@hack/do.sh bench

.PHONY: act
act: ## Test GitHub Actions workflows locally using 'act'
	hack/do.sh act

.PHONY: dev-deps
dev-deps: ## Install development dependencies (e.g., linters)
	@hack/do.sh dev_deps

##@ Code/Docs Generation
.PHONY: gen
gen: ## Run Go generate for code generation tasks
	@hack/do.sh gen

.PHONY: docs
docs: ## Generate project documentation
	@hack/do.sh docs

.PHONY: completions
completions: ## Generate shell completion scripts (bash, zsh, fish)
	@hack/do.sh completions

##@ Docker Operations
.PHONY: docker-build
docker-build: ## Build the Docker image locally
	@hack/do.sh docker_build

.PHONY: docker-push
docker-push: ## Build and push the Docker image to the registry
	@hack/do.sh docker_push

##@ Release Management
.PHONY: release-patch
release-patch: ## Create and tag a new patch release (e.g., v1.0.0 -> v1.0.1)
	@hack/do.sh release patch

.PHONY: release-minor
release-minor: ## Create and tag a new minor release (e.g., v1.0.0 -> v1.1.0)
	@hack/do.sh release minor

##@ Utility & Other
.PHONY: clean
clean: ## Remove generated binaries and Go build cache
	@hack/do.sh clean

.PHONY: build-nolint
build-nolint: ## Build the main binary without running linters first
	@NOLINT=1 hack/do.sh build

.PHONY: help
help: ## Show this help message listing all targets grouped by category
	@echo "------------------------------------------------------------------"
	@echo ""
	@awk 'BEGIN {FS = ":.*##"; printf "\033[31;01mUsage\033[0m: make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf " \033[36m%-25s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
