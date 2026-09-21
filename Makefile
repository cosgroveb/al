.DEFAULT_GOAL := build
.PHONY: help build test lint man clean _require-pandoc

PANDOC ?= pandoc
MAN_PAGE := build/docs/man/man1/al.1

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "; printf "Available targets:\n"} \
	/^[a-zA-Z0-9][a-zA-Z0-9_-]*:.*## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build ./al
	go build -trimpath -o al .

test: ## Run tests with race detection and shuffled order
	go test -race -shuffle=on ./...

lint: ## Run Go linters
	golangci-lint run ./...

man: $(MAN_PAGE) ## Generate the man page from the CLI reference

$(MAN_PAGE): docs/reference/al.1.md | _require-pandoc
	mkdir -p "$(dir $@)"
	"$(PANDOC)" --from=markdown-smart --to=man --standalone "$<" -o "$@"

_require-pandoc:
	@command -v "$(PANDOC)" >/dev/null 2>&1 || { \
		printf 'pandoc is required for make man\n' >&2; exit 1; \
	}

clean: ## Remove ./al and generated man pages
	rm -f al
	rm -rf build/docs
