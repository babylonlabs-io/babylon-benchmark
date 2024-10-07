DOCKER := $(shell which docker)
GIT_TOPLEVEL := $(shell git rev-parse --show-toplevel)

build-babylond:
	$(MAKE) -C $(GIT_TOPLEVEL)/submodules/babylon/contrib/images babylond

build-benchmark: build-babylond

start-benchmark-from-snapshot: stop-benchmark build-benchmark
	docker compose -f docker-compose.yml up -d

stop-benchmark:
	docker compose -f docker-compose.yml down
	rm -rf $(CURDIR)/babylond_data_master
	rm -rf $(CURDIR)/babylond_data_follower
