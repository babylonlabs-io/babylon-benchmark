# Help target
help:
	@echo "Available targets:"
	@echo ""
	@echo "  build                           Build the 'dgd' binary with the current settings."
	@echo "  build-babylond                  Build the 'babylond' binary from the submodules."
	@echo "  build-benchmark                 Build the benchmark environment, including 'babylond'."
	@echo "  start-benchmark-from-snapshot   Start the benchmark environment from a snapshot."
	@echo "  stop-benchmark                  Stop the benchmark environment and clean up data."
	@echo "  run-dgd                         Run the 'dgd' binary to generate data."
	@echo "  test                            Run unit tests for the benchmarking tool"
	@echo "  test-e2e                        Run e2e tests for the benchmarking tool"
	@echo "  help                            Display this help message."
	@echo ""
	@echo "Variables:"
	@echo "  BUILDDIR                        Directory where binaries are built (default: ./build)."
	@echo "  build_tags                      Build tags for Go builds."
	@echo "  build_args                      Additional build arguments."
	@echo "  VERSION                         Git version used for the build."
	@echo "  ldflags                         Linker flags for Go builds."
	@echo ""

DOCKER := $(shell which docker)
GIT_TOPLEVEL := $(shell git rev-parse --show-toplevel)
BUILDDIR ?= $(CURDIR)/build
GO_BIN := ${GOPATH}/bin

build_tags := $(BUILD_TAGS)
build_args := $(BUILD_ARGS)

VERSION := $(shell echo $(shell git describe --tags) | sed 's/^v//')

ldflags := $(LDFLAGS) -X github.com/babylonlabs-io/babylon-benchmark/lib/versioninfo.version=$(VERSION)


BUILD_TARGETS := build install
BUILD_FLAGS := --tags "$(build_tags)" --ldflags '$(ldflags)'

PACKAGES_E2E=$(shell go list ./... | grep '/e2e')

build-babylond:
	$(MAKE) -C $(GIT_TOPLEVEL)/submodules/babylon/contrib/images babylond

build-benchmark: build-babylond

start-benchmark-from-snapshot: stop-benchmark build-benchmark
	docker compose -f docker-compose.yml up -d
	./scripts/run-profiler.sh

stop-benchmark:
	docker compose -f docker-compose.yml down
	rm -rf $(CURDIR)/babylond_data_master
	rm -rf $(CURDIR)/babylond_data_follower


build: BUILD_ARGS := $(build_args) -o $(BUILDDIR)/dgd

$(BUILD_TARGETS): go.sum $(BUILDDIR)/
	go $@ -mod=readonly $(BUILD_FLAGS) $(BUILD_ARGS) ./cmd/datagen

$(BUILDDIR)/:
	mkdir -p $(BUILDDIR)/

run-dgd: build
	@./build/dgd generate --total-fps 5 --total-stakers 150

test-e2e:
	go test -mod=readonly -failfast -timeout=15m -v $(PACKAGES_E2E) -count=1

.PHONY: build help

###############################################################################
###                                Testing                                  ###
###############################################################################

test:
	go test -race ./...

e2e-test:
	go test ./e2e -v

.PHONY: test unit-test e2e-test