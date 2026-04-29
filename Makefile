BINARY  := ccswitch
PKG     := ./cmd/ccswitch
OUT     := bin/$(BINARY)
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: help build install run sandbox test test-v vet fmt tidy clean release dev

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk -F':.*?## ' '{printf "  %-12s %s\n", $$1, $$2}'

build: ## Compile to ./bin/ccswitch
	@mkdir -p bin
	go build -ldflags '$(LDFLAGS)' -o $(OUT) $(PKG)

install: ## go install into $$GOBIN (~/go/bin)
	go install -ldflags '$(LDFLAGS)' $(PKG)

run: ## go run with ARGS, e.g. make run ARGS="list"
	@go run $(PKG) $(ARGS)

sandbox: ## Run with isolated CCSWITCH_CONFIG_DIR (no risk to real data)
	@dir=$$(mktemp -d); echo "→ CCSWITCH_CONFIG_DIR=$$dir"; \
	CCSWITCH_CONFIG_DIR=$$dir go run $(PKG) $(ARGS)

test: ## Run tests
	go test ./...

test-v: ## Run tests verbose
	go test -v ./...

vet: ## go vet
	go vet ./...

fmt: ## gofmt all files
	gofmt -s -w .

tidy: ## go mod tidy
	go mod tidy

dev: vet test build ## vet + test + build

clean: ## Remove bin/
	rm -rf bin

release: ## Build stripped release binary
	@mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(OUT) $(PKG)
