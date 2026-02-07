.PHONY: all build test lint clean install run-parse run-inspect run-match run-apply sync

MODULE  := github.com/vinodhalaharvi/stencil
BINARY  := stencil
VERSION := 0.3.0

GO      := go
GOFLAGS := -v

all: tidy build test

## tidy: run go mod tidy
tidy:
	$(GO) mod tidy

## build: compile the stencil binary
build: tidy
	$(GO) build $(GOFLAGS) -o $(BINARY) .

## test: run all tests with verbose output
test:
	$(GO) test ./... -v -count=1

## test-short: run tests quietly
test-short:
	$(GO) test ./... -count=1

## lint: run go vet (skip structtag for Participle)
lint:
	$(GO) vet -structtag=false ./...

## clean: remove build artifacts
clean:
	rm -f $(BINARY)
	$(GO) clean -cache -testcache

## install: install to GOPATH/bin
install: build
	$(GO) install .

## run-parse: validate example .lift files
run-parse: build
	./$(BINARY) parse examples/enforce-ctx-timeout.lift
	./$(BINARY) parse examples/entity-service.lift

## run-inspect: inspect an example
run-inspect: build
	./$(BINARY) inspect examples/enforce-ctx-timeout.lift

## run-match: run matching against testdata
run-match: build
	./$(BINARY) match examples/enforce-ctx-timeout.lift --source testdata/bad_http_client.go

## run-apply: apply transformation to testdata
run-apply: build
	./$(BINARY) apply examples/enforce-ctx-timeout.lift --source testdata/bad_http_client.go

## version: show version
version: build
	./$(BINARY) version

## sync: sync files to local project (preserves .git)
sync:
	@if [ -z "$(DEST)" ]; then echo "Usage: make sync DEST=/path/to/stencil"; exit 1; fi
	@echo "Syncing to $(DEST) (preserving .git)..."
	rsync -av --exclude='.git' --exclude='stencil' . $(DEST)/
	@echo "Done. Run 'cd $(DEST) && git status' to see changes."

## help: show targets
help:
	@echo "Stencil v$(VERSION) — structural code matching and generation for Go"
	@echo ""
	@grep -E '^## ' Makefile | sed 's/## /  /'
