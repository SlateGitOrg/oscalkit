# oscalkit — single entry point for every quality gate.
# `make check` = format check + vet (type-check) + golangci-lint + race tests + coverage gate.

GO            ?= go
GOLANGCI_LINT ?= golangci-lint
COVER_MIN     ?= 80
export GOFLAGS ?= -p=3

# Core library packages whose coverage is gated (the CLI is tested end to end
# but reported separately).
CORE_PKGS := ./oscal/... ./schema/... ./resolve/... ./param/... ./check/... ./catdiff/... ./sspgen/... ./gaps/...
CORE_LIST := $(shell echo $(CORE_PKGS) | sed 's|\./||g; s|/\.\.\.||g; s| |,|g')
MODULE    := github.com/SlateGitOrg/oscalkit

.PHONY: check fmt-check fmt vet lint test cover cover-html build demo bench fuzz fetch-content loc clean

check: fmt-check vet lint test cover
	@echo "make check: all gates passed"

fmt-check:
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	@echo "fmt-check: ok"

fmt:
	gofmt -w .

vet:
	$(GO) vet ./...
	$(GO) build ./...
	@echo "vet/type-check: ok"

lint:
	$(GOLANGCI_LINT) run ./...

test:
	$(GO) test -race -count=1 ./...

# Coverage of the core packages, measured across all of their tests.
cover:
	@$(GO) test -count=1 -coverpkg=$(shell echo $(CORE_LIST) | sed 's|\([^,]*\)|$(MODULE)/\1|g') -coverprofile=coverage.out $(CORE_PKGS) >/dev/null
	@total=$$($(GO) tool cover -func=coverage.out | awk '/^total:/ {sub("%","",$$3); print $$3}'); \
	echo "core coverage: $$total% (minimum $(COVER_MIN)%)"; \
	awk -v t="$$total" -v m="$(COVER_MIN)" 'BEGIN { exit (t+0 >= m+0) ? 0 : 1 }' || { echo "coverage below $(COVER_MIN)%"; exit 1; }

cover-html: cover
	$(GO) tool cover -html=coverage.out -o coverage.html

build:
	$(GO) build -o bin/oscalkit ./cmd/oscalkit

demo: build
	./scripts/demo.sh

bench:
	$(GO) test -run '^$$' -bench . -benchmem -count=5 ./resolve/

# Short fuzzing session for every fuzz target (seed corpora also run in `make test`).
fuzz:
	$(GO) test -run '^$$' -fuzz '^FuzzGlobMatch$$' -fuzztime 20s ./resolve/
	$(GO) test -run '^$$' -fuzz '^FuzzDecode$$' -fuzztime 20s ./oscal/
	$(GO) test -run '^$$' -fuzz '^FuzzRender$$' -fuzztime 20s ./param/

# Re-download the pinned NIST content and verify it against committed checksums.
fetch-content:
	./scripts/fetch-nist-content.sh

# Physical lines, and code lines (excluding blank lines and // comment lines).
loc:
	@printf "non-test Go: %s lines, %s code lines\n" \
	  "$$(find . -name '*.go' ! -name '*_test.go' -not -path './.git/*' -exec cat {} + | wc -l | tr -d ' ')" \
	  "$$(find . -name '*.go' ! -name '*_test.go' -not -path './.git/*' -exec cat {} + | grep -cvE '^[[:space:]]*(//|$$)')"
	@printf "test Go:     %s lines, %s code lines\n" \
	  "$$(find . -name '*_test.go' -not -path './.git/*' -exec cat {} + | wc -l | tr -d ' ')" \
	  "$$(find . -name '*_test.go' -not -path './.git/*' -exec cat {} + | grep -cvE '^[[:space:]]*(//|$$)')"

clean:
	rm -rf bin out coverage.out coverage.html
