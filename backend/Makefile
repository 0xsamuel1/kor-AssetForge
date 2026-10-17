.PHONY: help test test-race coverage coverage-html coverage-total lint vet fmt clean

# Exclude performance tests from the default run, matching CI.
PKGS := $(shell go list ./... | grep -v /tests/performance)
COVERAGE_FILE := coverage.out
COVERAGE_HTML := coverage.html

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

test: ## Run the unit test suite
	go test $(PKGS)

test-race: ## Run the unit test suite with the race detector
	go test -race $(PKGS)

coverage: ## Run tests and write a coverage profile
	go test -covermode=atomic -coverprofile=$(COVERAGE_FILE) $(PKGS)

coverage-html: coverage ## Generate an HTML coverage report
	go tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "Coverage report written to $(COVERAGE_HTML)"

coverage-total: coverage ## Print the overall coverage percentage
	@go tool cover -func=$(COVERAGE_FILE) | tail -n 1

vet: ## Run go vet
	go vet $(PKGS)

lint: ## Run golangci-lint if available
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed, skipping"

fmt: ## Format Go source
	gofmt -w .

clean: ## Remove coverage artifacts
	rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)
