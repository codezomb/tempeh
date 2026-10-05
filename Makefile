# Uses the project-local Go toolchain in .toolchain (see bin/go). Nothing is installed on the host.
GO := ./bin/go

build:
	$(GO) build -o bin/tempeh ./cmd/tempeh

fmt:
	./bin/gofmt -l -w cmd internal

# fmtcheck is the non-mutating form for hooks and CI; fmt is the fixer.
fmtcheck:
	@out=$$(./bin/gofmt -l cmd internal); test -z "$$out" || { echo "gofmt needed: $$out"; exit 1; }

vet:
	$(GO) vet ./cmd/... ./internal/...

test:
	$(GO) test ./cmd/... ./internal/...

check: fmtcheck vet test build

.PHONY: build fmt fmtcheck vet test check
