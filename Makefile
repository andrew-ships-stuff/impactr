SHELL := /bin/sh

BASE ?= master
HEAD ?= HEAD
IMPACTR_ARGS ?=

.PHONY: run install model setup build test race vet check bench example clean help

run: ## Analyze HEAD against the merge base with master (override BASE/HEAD)
	go run ./cmd/impactr analyze --base "$(BASE)" --head "$(HEAD)" $(IMPACTR_ARGS)

install: ## Install impactr into GOBIN (or GOPATH/bin)
	go install ./cmd/impactr

model: ## Download and checksum-verify the optional embedded CPU model
	go run ./cmd/impactr model install

setup: install model ## Install the CLI and embedded model in one command

build: ## Build the local CLI
	go build -trimpath -o ./bin/impactr ./cmd/impactr

test: ## Run the offline test suite
	go test ./...

race: ## Run tests with the race detector
	go test -race ./...

vet: ## Run Go static analysis
	go vet ./...

check: test race vet ## Run all required verification

bench: ## Run deterministic benchmarks
	go test -run '^$$' -bench . -benchmem ./internal/analysis ./internal/graph ./internal/render

example: build ## Regenerate docs/images/example-impact.svg from the session-service fixture
	@tmp=$$(mktemp -d); \
	cp -R testdata/session-service/before/. "$$tmp/"; \
	git -C "$$tmp" init -q; \
	git -C "$$tmp" -c user.email=fixture@local -c user.name=fixture add -A; \
	git -C "$$tmp" -c user.email=fixture@local -c user.name=fixture commit -qm base; \
	rm -rf "$$tmp"/*; \
	cp -R testdata/session-service/after/. "$$tmp/"; \
	git -C "$$tmp" -c user.email=fixture@local -c user.name=fixture add -A; \
	git -C "$$tmp" -c user.email=fixture@local -c user.name=fixture commit -qm after; \
	(cd "$$tmp" && $(CURDIR)/bin/impactr analyze --base HEAD~1 --head HEAD --no-ai --output .impactr >/dev/null) && \
	cp "$$tmp/.impactr/impact.svg" docs/images/example-impact.svg; \
	rc=$$?; rm -rf "$$tmp"; exit $$rc

clean: ## Remove local build and analysis artifacts
	go clean

help: ## Show targets
	@awk 'BEGIN {FS = ":.*## "; print "Impactr targets:"} /^[a-zA-Z_-]+:.*## / {printf "  %-10s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
