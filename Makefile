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
BUILD_FLAGS := --tags "$(build_tags)" --ldflags '$(ldflags)' -race

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

.PHONY: build help
