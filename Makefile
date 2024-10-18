DOCKER := $(shell which docker)
GIT_TOPLEVEL := $(shell git rev-parse --show-toplevel)
BUILDDIR ?= $(CURDIR)/build
GO_BIN := ${GOPATH}/bin

build_tags := $(BUILD_TAGS)
build_args := $(BUILD_ARGS)
ldflags := $(LDFLAGS)

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


build: BUILD_ARGS := $(build_args) -o $(BUILDDIR)

$(BUILD_TARGETS): go.sum $(BUILDDIR)/
	go $@ -mod=readonly $(BUILD_FLAGS) $(BUILD_ARGS) ./...

$(BUILDDIR)/:
	mkdir -p $(BUILDDIR)/

.PHONY: build
