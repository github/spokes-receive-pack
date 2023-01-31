SHELL = /bin/sh

NW_TEST_RUNNERS = 12

GO := go
M  := $(shell printf "\033[34;1m▶\033[0m")

BIN := $(CURDIR)/bin

# APPVERSION can be used by an app's build target. It uses the git sha of HEAD.
APPVERSION := $(or $(shell git rev-parse HEAD 2>/dev/null),"unknown")

# Add items to this variable to add to the `clean` target:
CLEAN :=

# Generic Go binaries
GO_BINARIES := \
	$(BIN)/spokes-receive-pack-wrapper \
	$(BIN)/spokes-receive-pack-networked-wrapper

EXECUTABLES := \
	$(BIN)/spokes-receive-pack

CLEAN += $(EXECUTABLES)

.PHONY: FORCE

.PHONY: all
all: info
all: $(EXECUTABLES)

.PHONY: info
info:
	$(GO) version
	git --version

$(BIN):
	mkdir -p $(BIN)

###########################################################################

# Build binaries
#
# We need to compile using `go build` rather than `go install`,
# because the latter doesn't work for cross-compiling.

# Build the main service app:
$(BIN)/spokes-receive-pack: FORCE | $(BIN)
	$(GO) build -ldflags '-X main.BuildVersion=$(APPVERSION)' -o $@ .

# Build other generic Go binaries:
.PHONY: go-binaries
go-binaries: $(GO_BINARIES)
$(GO_BINARIES): $(BIN)/%: cmd/% FORCE | $(BIN)
	$(GO) build $(BUILDTAGS) -o $@ ./$<


###########################################################################

# Testing

.PHONY: test
test: go-test
test-integration: BUILDTAGS=-tags integration
test-integration: all go-binaries go-test-integration

TESTFLAGS := -race
TESTINTEGRATIONFLAGS := $(TESTFLAGS) --tags=integration --count=1
TESTSUITE := ./...
.PHONY: go-test
go-test:
	@echo "$(M) running tests..."
	$(GO) test $(TESTFLAGS) $(TESTSUITE) 2>&1

go-test-integration:
	@echo "$(M) running integration tests..."

	# Add our compiled `spokes-receive-pack` to the PATH while running tests:
	PATH="$(CURDIR)/bin:$(PATH)" \
	    $(GO) test $(TESTINTEGRATIONFLAGS) $(TESTSUITE) 2>&1

CLEAN += log/*.log

###########################################################################

# Benchmarks

BENCHFLAGS :=
bench:
	@echo "$(M) running benchmarks..."
	$(GO) test -bench=. $(BENCHFLAGS) $(TESTSUITE) 2>&1

###########################################################################

# Miscellaneous

.PHONY: coverage
coverage:
	@echo "$(M) running code coverage..."
	$(GO) test $(TESTFLAGS) $(TESTSUITE) -coverprofile coverage.out 2>&1
	$(GO) tool cover -html=coverage.out
	rm -f coverage.out

# Profiling
PPROF := $(BIN)/pprof
$(PPROF):
	$(GO) get -u github.com/google/pprof

.PHONY: pprof
pprof: | $(PPROF) ## Build the pprof binary

# Formatting
GOFMT := $(BIN)/goimports
$(BIN)/goimports:
	GOBIN=$(BIN) $(GO) install golang.org/x/tools/cmd/goimports

# Run goimports on all source files:
.PHONY: fmt
fmt: | $(GOFMT)
	@echo "$(M) running goimports..."
	@ret=0 && for d in $$($(GO) list -f '{{.Dir}}' ./...); do \
		$(GOFMT) -l -w $$d/*.go || ret=$$? ; \
	done ; exit $$ret


# Run golang-ci lint on all source files:
GOLANGCILINT := $(BIN)/golangci-lint
$(BIN)/golangci-lint:
	GOBIN=$(BIN) $(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

.PHONY: fmt
lint: | $(GOLANGCILINT)
	@echo "$(M) running golangci-lint"
	$(GOLANGCILINT) run

###########################################################################

# Cleanup

.PHONY: clean
clean:
	@echo "$(M) cleaning..."
	rm -f $(CLEAN)
