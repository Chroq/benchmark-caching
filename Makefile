# ============================================================================
# CACHE BENCHMARK AUTOMATION MAKEFILE
# ============================================================================
# Usage:
#   make          - Runs the full sequential benchmark (600 seconds per run)
#   make quick    - Runs a quick 5-second sanity check benchmark
#   make tune-os  - Tunes Linux network socket parameters to avoid TIME_WAIT exhaustion
#   make clean    - Removes built binaries
# ============================================================================

# Default duration for the bombardier tests (can be overridden on CLI)
DURATION ?= 600s
CONNECTIONS ?= 250
WARMUP_DURATION ?= 30s

# Default environment variables
PORT ?= 8080
DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable
VALKEY_URL ?= localhost:6379
LOG_LEVEL ?= production

export PORT
export DATABASE_URL
export VALKEY_URL
export LOG_LEVEL

.PHONY: all build clean run-all quick medium long bench-memory bench-valkey bench-standard-postgres bench-opt-postgres tune-os

# Run the complete test suite sequentially
all: build run-all

# Run a quick 5-second test suite
quick:
	$(MAKE) DURATION=5s WARMUP_DURATION=2s

# Run a medium 60-second test suite
medium:
	$(MAKE) DURATION=60s WARMUP_DURATION=10s

# Run a long 10-minute test suite
long:
	$(MAKE) DURATION=600s WARMUP_DURATION=30s

# Compile production-optimized binary
build:
	@echo "=== Compiling Go Benchmark Server ==="
	@mkdir -p bin
	@go build -ldflags="-s -w" -o bin/server ./cmd/main.go
	@echo "Build successful: ./bin/server"

# Clean up build artifacts
clean:
	@echo "=== Cleaning Up Binaries ==="
	@rm -rf bin
	@echo "Done."

# Tune Linux OS network TCP socket recycling parameters for high-throughput load tests
tune-os:
	@echo "=== Tuning Linux OS Network Parameters for High Concurrency ==="
	sudo sysctl -w net.ipv4.tcp_tw_reuse=1
	sudo sysctl -w net.ipv4.ip_local_port_range="1024 65535"
	@echo "OS network tuning complete."

# Master orchestrator running benchmarks sequentially
run-all:
	@echo "========================================================"
	@echo " STARTING SEQUENTIAL CACHE BENCHMARK"
	@echo " Concurrency: $(CONNECTIONS) connections | Duration: $(DURATION)"
	@echo " Server Limits: 4 CPU (GOMAXPROCS) | 4G RAM (GOMEMLIMIT)"
	@echo "========================================================"
	@$(call run_bench,memory,memory)
	@$(call run_bench,valkey,valkey)
	@$(call run_bench,standard-postgresql,postgres)
	@$(call run_bench,optimized-postgresql,postgres)
	@echo "========================================================"
	@echo " ALL BENCHMARKS COMPLETED SUCCESSFULLY"
	@echo "========================================================"

# Specific targets to run individual tests if needed
bench-memory: build
	@$(call run_bench,memory,memory)

bench-valkey: build
	@$(call run_bench,valkey,valkey)

bench-opt-postgres: build
	@$(call run_bench,optimized-postgresql,postgres)

bench-standard-postgres: build
	@$(call run_bench,standard-postgresql,postgres)

# Benchmark direct PostgreSQL calls (Go driver level reference benchmark)
bench-postgres-direct:
	@echo "=== Running Direct PostgreSQL Driver Benchmarks ==="
	@go test -v -bench=BenchmarkPostgresDirect -run=^$$ ./adapters/postgres/...

# Helper to stop postgresql / valkey services
define manage_services
	if [ "$(1)" = "memory" ]; then \
		echo "Stopping PostgreSQL and Valkey for memory isolation..." ; \
		sudo systemctl stop postgresql postgres valkey-server valkey redis-server 2>/dev/null || true ; \
		sleep 1 ; \
	elif [ "$(1)" = "valkey" ]; then \
		echo "Stopping PostgreSQL and starting Valkey..." ; \
		sudo systemctl stop postgresql postgres 2>/dev/null || true ; \
		sudo systemctl start valkey-server valkey redis-server 2>/dev/null || true ; \
		sleep 2 ; \
	else \
		echo "Stopping Valkey and starting PostgreSQL..." ; \
		sudo systemctl stop valkey-server valkey redis-server 2>/dev/null || true ; \
		sudo systemctl start postgresql postgres 2>/dev/null || true ; \
		sleep 2 ; \
	fi
endef

# Reusable macro to safely launch the server, wait for health check, run tests, and cleanup
define run_bench
	echo ""
	echo "========================================================"
	echo " ENGINE: $(1) "
	echo "========================================================"
	@$(MAKE) tune-os
	$(call manage_services,$(1))
	echo "Ensuring port $(PORT) is clear..." ; \
	pids=$$(lsof -t -i:$(PORT) 2>/dev/null) ; if [ -n "$$pids" ]; then kill -9 $$pids 2>/dev/null || true ; fi ; \
	echo "Starting server with GOMAXPROCS=4 and GOMEMLIMIT=4GiB..." ; \
	GOMAXPROCS=4 GOMEMLIMIT=4GiB ./bin/server -engine $(1) -log-level $(LOG_LEVEL) & pid=$$! ; \
	echo "Waiting for server (PID $$pid) to be ready on port $(PORT)..." ; \
	for i in $$(seq 1 1200); do \
		if curl -s http://localhost:$(PORT)/health >/dev/null; then \
			break; \
		fi; \
		sleep 0.5; \
	done; \
	if ! curl -s http://localhost:$(PORT)/health >/dev/null; then \
		echo "ERROR: Server failed to start." ; \
		kill $$pid 2>/dev/null || true ; \
		exit 1 ; \
	fi ; \
	if [ "$(1)" = "optimized-postgresql" ] || [ "$(1)" = "standard-postgresql" ]; then \
		echo "Warm-up phase: préchauffage de PostgreSQL ($(WARMUP_DURATION))..." ; \
		bombardier -q -c $(CONNECTIONS) -d $(WARMUP_DURATION) http://localhost:$(PORT)/$(2)/get ; \
	fi ; \
	echo "Executing GET benchmark..." ; \
	bombardier -c $(CONNECTIONS) -d $(DURATION) -l http://localhost:$(PORT)/$(2)/get ; \
	echo "Executing SET benchmark..." ; \
	bombardier -m POST -l -f payload.json -c $(CONNECTIONS) -d $(DURATION) http://localhost:$(PORT)/$(2)/set ; \
	echo "Stopping server (PID $$pid)..." ; \
	kill $$pid 2>/dev/null || true ; \
	wait $$pid 2>/dev/null || true ; \
	echo "Finished $(1) benchmark." ; \
	pids=$$(lsof -t -i:$(PORT) 2>/dev/null) ; if [ -n "$$pids" ]; then kill -9 $$pids 2>/dev/null || true ; fi
endef
