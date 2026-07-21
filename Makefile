.PHONY: help test test-race test-coverage coverage-summary bench lint fmt vet docs check clean install-tools

.DEFAULT_GOAL := help

# Go parameters
GOCMD=go
GOTEST=$(GOCMD) test
GOBUILD=$(GOCMD) build
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet
GOMOD=$(GOCMD) mod

# Coverage
COVERAGE_DIR=coverage
COVERAGE_FILE=$(COVERAGE_DIR)/coverage.out
COVERAGE_HTML=$(COVERAGE_DIR)/coverage.html

# Packages
PACKAGES=./...

# Colors for terminal output
GREEN=\033[0;32m
YELLOW=\033[0;33m
RED=\033[0;31m
NC=\033[0m # No Color

help:
	@echo "rhttp - Production-grade HTTP client for Go"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@sed -n 's/^##//p' $(MAKEFILE_LIST) | column -t -s ':' | sed -e 's/^/ /'

test:
	@echo "$(GREEN)Running tests...$(NC)"
	$(GOTEST) -v $(PACKAGES)

test-race:
	@echo "$(GREEN)Running tests with race detector...$(NC)"
	$(GOTEST) -v -race $(PACKAGES)

test-coverage:
	@echo "$(GREEN)Running tests with coverage...$(NC)"
	@mkdir -p $(COVERAGE_DIR)
	$(GOTEST) -v -coverprofile=$(COVERAGE_FILE) -covermode=atomic $(PACKAGES)
	$(GOCMD) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	$(GOCMD) tool cover -func=$(COVERAGE_FILE)
	@echo ""
	@echo "$(GREEN)Coverage report generated: $(COVERAGE_HTML)$(NC)"

coverage-summary:
	@echo "$(GREEN)=== Total Coverage ===$(NC)"
	@$(GOCMD) tool cover -func=$(COVERAGE_FILE) | tail -1
	@echo ""
	@echo "$(YELLOW)=== Uncovered Functions (0.0%) ===$(NC)"
	@$(GOCMD) tool cover -func=$(COVERAGE_FILE) | awk '$$NF == "0.0%"'

test-short:
	@echo "$(GREEN)Running short tests...$(NC)"
	$(GOTEST) -v -short $(PACKAGES)

bench:
	@echo "$(GREEN)Running benchmarks...$(NC)"
	$(GOTEST) -bench=. -benchmem $(PACKAGES)

bench-compare:
	@echo "$(GREEN)Running benchmarks for comparison...$(NC)"
	$(GOTEST) -bench=. -benchmem -count=5 $(PACKAGES) | tee benchmarks.txt

lint:
	@echo "$(GREEN)Running linter...$(NC)"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run $(PACKAGES); \
	else \
		echo "$(YELLOW)golangci-lint not installed. Run 'make install-tools' first.$(NC)"; \
		exit 1; \
	fi

fmt:
	@echo "$(GREEN)Formatting code...$(NC)"
	$(GOFMT) $(PACKAGES)

fmt-check:
	@echo "$(GREEN)Checking code formatting...$(NC)"
	@test -z "$$(gofmt -l .)" || (echo "$(RED)Code is not formatted. Run 'make fmt'$(NC)" && gofmt -l . && exit 1)

vet:
	@echo "$(GREEN)Running go vet...$(NC)"
	$(GOVET) $(PACKAGES)

docs:
	@echo "$(GREEN)Starting documentation server...$(NC)"
	@echo "Open http://localhost:8080/github.com/oswaldom-code/rhttp"
	@if command -v pkgsite >/dev/null 2>&1; then \
		pkgsite -http=:8080; \
	else \
		echo "$(YELLOW)pkgsite not installed. Run 'make install-tools' first.$(NC)"; \
		echo "Falling back to godoc..."; \
		godoc -http=:8080; \
	fi

check: fmt-check vet lint test
	@echo "$(GREEN)All checks passed!$(NC)"

check-all: fmt-check vet lint test-race
	@echo "$(GREEN)All checks passed!$(NC)"

clean:
	@echo "$(GREEN)Cleaning...$(NC)"
	@rm -rf $(COVERAGE_DIR)
	@rm -f benchmarks.txt
	@rm -f benchmark_results.txt
	$(GOCMD) clean -cache -testcache


install-tools:
	@echo "$(GREEN)Installing development tools...$(NC)"
	@echo "Installing golangci-lint..."
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "Installing pkgsite..."
	go install golang.org/x/pkgsite/cmd/pkgsite@latest
	@echo "$(GREEN)Done! Make sure $(GOPATH)/bin is in your PATH.$(NC)"

ci: deps fmt-check vet lint test-race
	@echo "$(GREEN)CI pipeline passed!$(NC)"

version:
	@$(GOCMD) version

info:
	@echo "Module: $$(head -1 go.mod | cut -d' ' -f2)"
	@echo "Go version: $$($(GOCMD) version | cut -d' ' -f3)"
	@echo "Packages: $$($(GOCMD) list $(PACKAGES) | wc -l | tr -d ' ')"
	@echo "Test files: $$(find . -name '*_test.go' | wc -l | tr -d ' ')"
	@echo "Source files: $$(find . -name '*.go' ! -name '*_test.go' | wc -l | tr -d ' ')"
