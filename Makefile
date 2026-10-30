.PHONY: test test-live test-all fmt lint vet

# Offline tests only (no network): client unit tests with httptest, helper tests.
test:
	go test -race -count=1 ./...

# Live tests against the public API (build tag "live"). Override the target with GRAPHQL_API_URL.
test-live:
	go test -race -count=1 -tags live ./tests/...

test-all:
	go test -race -count=1 -tags live ./...

fmt:
	gofmt -w .

vet:
	go vet ./...
	go vet -tags live ./...

# gofmt check always; golangci-lint too when installed.
lint: vet
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	@if command -v golangci-lint >/dev/null 2>&1; then golangci-lint run ./... ; else echo "golangci-lint not installed, skipped"; fi
