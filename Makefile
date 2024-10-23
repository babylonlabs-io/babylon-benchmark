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
	@./build/dgd generate

.PHONY: build
