# ── Variables ─────────────────────────────────────────────────────────────────
BINARY        := control-plane-server
CMD           := ./cmd/server
BUILD_DIR     := ./bin
VERSION       ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS       := -s -w -X main.Version=$(VERSION)

# Docker Compose file relative to this Makefile.
COMPOSE_FILE  := deployments/docker-compose.yml

# Database connection string for the migrate target (override via env).
DATABASE_URL  ?= postgres://grid:gridpass@localhost:5432/griddb?sslmode=disable

# Platforms for cross-compile.
PLATFORMS     := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

.PHONY: all build test lint migrate docker-up docker-down generate-key cross-compile clean

# ── Default target ────────────────────────────────────────────────────────────
all: build

# ── Build ─────────────────────────────────────────────────────────────────────
build:
	@echo "==> Building $(BINARY)..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build \
		-trimpath \
		-ldflags "$(LDFLAGS)" \
		-o $(BUILD_DIR)/$(BINARY) \
		$(CMD)
	@echo "==> Binary at $(BUILD_DIR)/$(BINARY)"

# ── Test ──────────────────────────────────────────────────────────────────────
test:
	@echo "==> Running tests..."
	go test -race -timeout 120s ./...

# ── Lint ──────────────────────────────────────────────────────────────────────
lint:
	@echo "==> Running linters..."
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "golangci-lint not found – installing..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	}
	golangci-lint run ./...

# ── Migrate ───────────────────────────────────────────────────────────────────
# Requires github.com/golang-migrate/migrate CLI to be installed.
migrate:
	@echo "==> Running database migrations against $(DATABASE_URL)..."
	@command -v migrate >/dev/null 2>&1 || { \
		echo "migrate CLI not found – install from https://github.com/golang-migrate/migrate"; \
		exit 1; \
	}
	migrate -path migrations -database "$(DATABASE_URL)" up

# ── Docker Compose ────────────────────────────────────────────────────────────
docker-up:
	@echo "==> Starting services..."
	docker compose -f $(COMPOSE_FILE) up --build -d

docker-down:
	@echo "==> Stopping services..."
	docker compose -f $(COMPOSE_FILE) down

# ── Generate API key ──────────────────────────────────────────────────────────
# Runs a small Go program inline to produce a new key for local testing.
generate-key:
	@echo "==> Generating sample API key..."
	@go run - <<'EOF'
package main

import (
	"fmt"
	"github.com/grid-computing/control-plane/internal/security"
)

func main() {
	pt, prefix, hash, err := security.GenerateAPIKey()
	if err != nil {
		panic(err)
	}
	fmt.Printf("Plaintext : %s\n", pt)
	fmt.Printf("Prefix    : %s\n", prefix)
	fmt.Printf("Hash      : %s\n", hash)
}
EOF

# ── Cross-compile ─────────────────────────────────────────────────────────────
cross-compile:
	@echo "==> Cross-compiling for all platforms..."
	@mkdir -p $(BUILD_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$$(echo $$platform | cut -d'/' -f1); \
		GOARCH=$$(echo $$platform | cut -d'/' -f2); \
		output="$(BUILD_DIR)/$(BINARY)-$$GOOS-$$GOARCH"; \
		if [ "$$GOOS" = "windows" ]; then output="$$output.exe"; fi; \
		echo "  -> $$GOOS/$$GOARCH => $$output"; \
		CGO_ENABLED=0 GOOS=$$GOOS GOARCH=$$GOARCH \
			go build -trimpath -ldflags "$(LDFLAGS)" -o $$output $(CMD); \
	done
	@echo "==> Cross-compile complete."

# ── Clean ─────────────────────────────────────────────────────────────────────
clean:
	@echo "==> Cleaning build artefacts..."
	@rm -rf $(BUILD_DIR)
