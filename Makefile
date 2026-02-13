# Makefile
BINARY_NAME := $(shell basename $(CURDIR))
MODULE := $(shell head -1 go.mod | awk '{print $$2}')
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(BUILD_DATE)"

# Conditionally include docs targets if directory exists
ALL_TARGETS := build
$(if $(wildcard docs/mkdocs.yml),$(eval ALL_TARGETS += docs-build))

.DEFAULT_GOAL := all

##@ App
.PHONY: build install run clean tidy test lint vet fmt

build: ## Build the Go binary
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) .

install: ## Install the binary to $GOPATH/bin
	go install $(LDFLAGS) .

run: build ## Build and run the binary
	./bin/$(BINARY_NAME)

clean: ## Remove build artifacts
	rm -rf bin/
	rm -f coverage.out

tidy: ## Run go mod tidy
	go mod tidy

test: ## Run tests
	go test -v -race -count=1 ./...

test-cover: ## Run tests with coverage
	go test -v -race -count=1 -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

lint: vet ## Run golangci-lint
	@which golangci-lint > /dev/null 2>&1 || { echo "Install golangci-lint: https://golangci-lint.run/welcome/install/"; exit 1; }
	golangci-lint run ./...

vet: ## Run go vet
	go vet ./...

fmt: ## Run gofmt
	gofmt -s -w .

##@ Release
.PHONY: release release-local release-snapshot

release: ## Create a release with goreleaser
	goreleaser release --clean

release-local: ## Create a signed local release (macOS code-signing)
	goreleaser release --clean

release-snapshot: ## Create a snapshot release (no publish)
	goreleaser release --snapshot --clean --skip homebrew

##@ Docs (mkdocs-material via uv)
.PHONY: docs-serve docs-build docs-deps

docs-serve: ## Serve docs locally (requires uv + docs/ directory)
	@[ -d docs ] && [ -f docs/mkdocs.yml ] || { echo "No docs/ directory with mkdocs.yml found."; exit 1; }
	cd docs && uv run mkdocs serve

docs-build: ## Build docs site (requires uv + docs/ directory)
	@[ -d docs ] && [ -f docs/mkdocs.yml ] || { echo "No docs/ directory with mkdocs.yml found."; exit 1; }
	cd docs && uv run mkdocs build

docs-deps: ## Install doc dependencies (requires uv + docs/ directory)
	@[ -d docs ] && [ -f docs/pyproject.toml ] || { echo "No docs/ directory with pyproject.toml found."; exit 1; }
	cd docs && uv sync

##@ All
.PHONY: all deps

all: $(ALL_TARGETS) ## Build all existing artifacts (app + docs)

deps: tidy ## Install all dependencies
	@[ -d docs ] && [ -f docs/pyproject.toml ] && (cd docs && uv sync) || true

##@ Help
.PHONY: help

help: ## Show this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) }' $(MAKEFILE_LIST)
