GOFMT ?= gofmt
GOVET ?= go vet
GOTEST ?= go test

.PHONY: fmt vet test check quick

# Format all Go sources in the repository.
fmt:
	$(GOFMT) -w .

# Run go vet to catch common issues.
vet:
	$(GOVET) ./...

# Run the full test suite.
test:
	$(GOTEST) ./...

# Convenience target to format, vet, and test in one go.
check: fmt vet test

# Lightweight target for quick iteration: vet and unit tests without reformatting.
quick:
	$(GOVET) ./...
	$(GOTEST) ./...
