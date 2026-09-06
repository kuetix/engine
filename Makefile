MCP_PATH   := ./cmd/mcp-server
MCP_SERVER := mcp-server
BUILD_DIR  := runtime/bin
DIST_DIR   := dist
INSTALL_DIR := $(or $(GOBIN),$(if $(GOPATH),$(GOPATH)/bin,$(if $(HOME),$(HOME)/go/bin,/usr/local/bin)))

VERSION    := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")
BUILD_TIME := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS    := -s -w -X 'main.Version=$(VERSION)' -X 'main.BuildTime=$(BUILD_TIME)'

# Cross-compile matrix for `make build-all` (goreleaser handles releases).
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

GO         ?= go
GOLANGCI   ?= golangci-lint
ACTIONLINT ?= actionlint
GORELEASER ?= goreleaser

.DEFAULT_GOAL := help

help: ## Display this help message
	@echo "Available targets:"
	@grep -E '^[0-9a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

## --- Build ---

build: ## Build the MCP server binary
	$(GO) build -ldflags "$(LDFLAGS)" -o $(MCP_SERVER) $(MCP_PATH)

build-all: ## Cross-compile the MCP server for every target in PLATFORMS -> dist/
	@mkdir -p $(DIST_DIR)
	@for p in $(PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=""; \
		[ "$$os" = "windows" ] && ext=".exe"; \
		out="$(DIST_DIR)/$(MCP_SERVER)_$${os}_$${arch}$${ext}"; \
		echo "  $$out"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o "$$out" $(MCP_PATH) || exit 1; \
	done

build_linux_arm: ## Build the MCP server binary for Linux ARM64
	GOARCH="arm64" GOOS="linux" CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o "$(MCP_SERVER)_arm64" $(MCP_PATH)

install: ## Install the MCP server to $(INSTALL_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o "$(INSTALL_DIR)/$(MCP_SERVER)" $(MCP_PATH)

run: build ## Build and run the MCP server
	./$(MCP_SERVER)

## --- Quality / CI ---

fmt: ## Format Go code
	$(GO) fmt ./...

fmt-check: ## Fail if any Go file is not gofmt-clean
	@out=$$(gofmt -l $$(git ls-files '*.go' | grep -v '^vendor/')); \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

vet: ## Run go vet
	$(GO) vet ./...

lint: ## Run golangci-lint (skipped if not installed)
	@command -v $(GOLANGCI) >/dev/null 2>&1 && $(GOLANGCI) run ./... || echo "golangci-lint not installed - skipping"

actionlint: ## Lint GitHub Actions workflows (skipped if not installed)
	@command -v $(ACTIONLINT) >/dev/null 2>&1 && $(ACTIONLINT) || echo "actionlint not installed - skipping"

test: ## Run all tests
	$(GO) test ./...

test-race: ## Run all tests with the race detector
	$(GO) test -race ./...

cover: ## Run tests with coverage and print a summary
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -1

ci: fmt-check vet test build actionlint ## Run the full CI gate locally

## --- Release ---

tag: ## Create an annotated release tag (usage: make tag TAG=v0.x.y)
	@if [ -z "$(TAG)" ]; then echo "Usage: make tag TAG=v0.x.y"; exit 1; fi
	@case "$(TAG)" in v[0-9]*) ;; *) echo "TAG must look like vX.Y.Z"; exit 1;; esac
	@if [ -n "$$(git status --porcelain)" ]; then echo "working tree not clean"; exit 1; fi
	git tag -a $(TAG) -m "Release $(TAG)"
	@echo "Created tag $(TAG). Push it with:  git push origin $(TAG)"
	@echo "The release workflow builds and publishes the GitHub Release."

snapshot: ## Build a local release snapshot with goreleaser (no publish)
	$(GORELEASER) release --snapshot --clean --skip=publish

release-check: ## Validate the goreleaser config
	$(GORELEASER) check

release: ## Run goreleaser (used by CI on a tag; needs GITHUB_TOKEN)
	$(GORELEASER) release --clean

## --- Housekeeping ---

deps: ## Tidy Go module dependencies
	$(GO) mod tidy

clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR) $(DIST_DIR) $(MCP_SERVER) $(MCP_SERVER)_arm64 coverage.out

.PHONY: help build build-all build_linux_arm install run \
	fmt fmt-check vet lint actionlint test test-race cover ci \
	tag snapshot release-check release deps clean
