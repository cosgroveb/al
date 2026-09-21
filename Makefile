.DEFAULT_GOAL := build
.PHONY: help build test lint clean

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "; printf "Available targets:\n"} \
	/^[a-zA-Z0-9][a-zA-Z0-9_-]*:.*## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build ./al
	go build -trimpath -o al .

test: ## Run tests with race detection and shuffled order
	go test -race -shuffle=on ./...

lint: ## Run Go linters
	golangci-lint run ./...

clean: ## Remove ./al
	rm -f al
