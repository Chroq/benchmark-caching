# ============================================================================
# CACHE BENCHMARK AUTOMATION MAKEFILE (VEGETA Edition)
# ============================================================================
# Usage:
#   make                      - Runs the full sequential benchmark (stdout)
#   make quick                - Runs a quick 5-second check (stdout)
#   make quick OUT_FILE=f.txt - Runs quick benchmark and saves clean results to f.txt
#   make clean-bench          - Cleans existing raw bench_*.txt log files
#   make tune-os              - Tunes Linux network socket parameters
#   make clean                - Removes built binaries and target files
# ============================================================================

# Default duration for the vegeta tests (can be overridden on CLI)
DURATION ?= 600s
CONNECTIONS ?= 250
WARMUP_DURATION ?= 30s

# Optional output file argument (alias FILE or OUT)
TARGET_OUT ?= $(OUT_FILE)$(FILE)$(OUT)

# Default environment variables
PORT ?= 8080
DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable
VALKEY_URL ?= localhost:6379
LOG_LEVEL ?= production
VEGETA ?= $(shell which vegeta 2>/dev/null || echo $(HOME)/go/bin/vegeta)

export PORT
export DATABASE_URL
export VALKEY_URL
export LOG_LEVEL

.PHONY: all build clean run-all quick medium long bench-memory bench-valkey bench-standard-postgres bench-opt-postgres gen-targets clean-bench tune-os

# Run the complete test suite sequentially (600s default)
all:
	@$(call run_suite,$(DURATION),$(WARMUP_DURATION))

# Run a quick 5-second test suite
quick:
	@$(call run_suite,5s,2s)

# Run a medium 300-second test suite
medium:
	@$(call run_suite,300s,10s)

# Run a long 10-minute test suite
long:
	@$(call run_suite,600s,30s)

# Compile production-optimized binary
build:
	@echo "=== Compiling Go Benchmark Server ==="
	@mkdir -p bin
	@go build -ldflags="-s -w" -o bin/server ./cmd/benchmark
	@echo "Build successful: ./bin/server"

# Generate ULID/UUID keys and Vegeta attack target files
gen-targets:
	@echo "=== Generating ULID Keys and Vegeta Targets in gen/ ==="
	@mkdir -p gen
	@go run ./cmd/gentargets -count=100000 -port=$(PORT) -keys-file=gen/keys.txt -targets-dir=gen

# Macro to run benchmark suites: outputs to stdout by default, or cleans output into TARGET_OUT if specified
define run_suite
	if [ -n "$(TARGET_OUT)" ]; then \
		echo "=== Running benchmark suite -> saving cleaned metrics to $(TARGET_OUT) ===" ; \
		tmp_raw=$$(mktemp) ; \
		$(MAKE) --no-print-directory build gen-targets run-all DURATION=$(1) WARMUP_DURATION=$(2) > "$$tmp_raw" 2>&1 || true ; \
		awk 'BEGIN { print "========================================================"; print " CLEANED BENCHMARK RESULTS"; print "========================================================" } /ENGINE:/ { print "\n========================================================"; print $$0; print "========================================================"; next } /Executing (GET|SET) benchmark/ || /Warm-up phase/ { print "\n--- " $$0 " ---"; next } /^(Requests|Duration|Latencies|Bytes In|Bytes Out|Success|Status Codes)/ { print $$0 }' "$$tmp_raw" > "$(TARGET_OUT)" ; \
		rm -f "$$tmp_raw" ; \
		echo "Cleaned benchmark output successfully saved to $(TARGET_OUT)" ; \
	else \
		$(MAKE) --no-print-directory build gen-targets run-all DURATION=$(1) WARMUP_DURATION=$(2) ; \
	fi
endef

# Strip superfluous error logs and output clean benchmark metrics files
clean-bench:
	@echo "=== Cleaning Benchmark Output Files ==="
	@if [ -f "$(TARGET_OUT)" ]; then \
		out_file=$$(echo "$(TARGET_OUT)" | sed 's/\.txt$$//')_clean.txt ; \
		echo "Cleaning $(TARGET_OUT) -> $$out_file..." ; \
		awk 'BEGIN { print "========================================================"; print " CLEANED BENCHMARK RESULTS"; print "========================================================" } /ENGINE:/ { print "\n========================================================"; print $$0; print "========================================================"; next } /Executing (GET|SET) benchmark/ || /Warm-up phase/ { print "\n--- " $$0 " ---"; next } /^(Requests|Duration|Latencies|Bytes In|Bytes Out|Success|Status Codes)/ { print $$0 }' "$(TARGET_OUT)" > "$$out_file" ; \
		echo "Cleaned result written to $$out_file" ; \
	else \
		echo "Error: File $(TARGET_OUT) not found." ; \
		exit 1 ; \
	fi ;

# Clean up build artifacts
clean:
	@echo "=== Cleaning Up Binaries & Target Files ==="
	@rm -rf bin gen
	@echo "Done."

# Tune Linux OS network TCP socket recycling parameters for high-throughput load tests
tune-os:
	@echo "=== Tuning Linux OS Network Parameters for High Concurrency ==="
	@ulimit -n 65535 2>/dev/null || true
	@sudo -n sysctl -w net.ipv4.tcp_tw_reuse=1 2>/dev/null || true
	@sudo -n sysctl -w net.ipv4.ip_local_port_range="1024 65535" 2>/dev/null || true
	@sudo -n sysctl -w net.core.somaxconn=65535 2>/dev/null || true
	@sudo -n sysctl -w net.ipv4.tcp_max_syn_backlog=65535 2>/dev/null || true
	@echo "OS network tuning complete."

# Master orchestrator running benchmarks sequentially
run-all:
	@echo "========================================================"
	@echo " STARTING SEQUENTIAL CACHE BENCHMARK (VEGETA)"
	@echo " Concurrency: $(CONNECTIONS) workers | Duration: $(DURATION)"
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
bench-memory: build gen-targets
	@$(call run_bench,memory,memory)

bench-valkey: build gen-targets
	@$(call run_bench,valkey,valkey)

bench-opt-postgres: build gen-targets
	@$(call run_bench,optimized-postgresql,postgres)

bench-standard-postgres: build gen-targets
	@$(call run_bench,standard-postgresql,postgres)

# Helper to stop postgresql / valkey services
define manage_services
	if [ "$(1)" = "memory" ]; then \
		echo "Stopping PostgreSQL and Valkey for memory isolation..." ; \
		sudo -n systemctl stop postgresql postgres valkey-server valkey redis-server 2>/dev/null || true ; \
		sleep 1 ; \
	elif [ "$(1)" = "valkey" ]; then \
		echo "Stopping PostgreSQL and starting Valkey..." ; \
		sudo -n systemctl stop postgresql postgres 2>/dev/null || true ; \
		sudo -n systemctl start valkey-server valkey redis-server 2>/dev/null || true ; \
		sleep 2 ; \
	else \
		echo "Stopping Valkey and starting PostgreSQL..." ; \
		sudo -n systemctl stop valkey-server valkey redis-server 2>/dev/null || true ; \
		sudo -n systemctl start postgresql postgres 2>/dev/null || true ; \
		sleep 2 ; \
	fi
endef

# Reusable macro to safely launch the server, wait for health check, run Vegeta tests, and cleanup
define run_bench
	echo ""
	echo "========================================================"
	echo " ENGINE: $(1) "
	echo "========================================================"
	@$(MAKE) tune-os
	$(call manage_services,$(1))
	echo "Ensuring port $(PORT) is clear..." ; \
	pids=$$(lsof -t -i:$(PORT) 2>/dev/null) ; if [ -n "$$pids" ]; then kill -9 $$pids 2>/dev/null || true ; fi ; \
	echo "Starting server with GOMAXPROCS=4 and GOMEMLIMIT=8GiB..." ; \
	GOMAXPROCS=4 GOMEMLIMIT=8GiB ./bin/server -engine $(1) -log-level $(LOG_LEVEL) & pid=$$! ; \
	echo "Waiting for server (PID $$pid) to be ready on port $(PORT)..." ; \
	for i in $$(seq 1 1200); do \
		if curl -s http://127.0.0.1:$(PORT)/health >/dev/null; then \
			break; \
		fi; \
		sleep 0.5; \
		done; \
	if ! curl -s http://127.0.0.1:$(PORT)/health >/dev/null; then \
		echo "ERROR: Server failed to start." ; \
		kill $$pid 2>/dev/null || true ; \
		exit 1 ; \
	fi ; \
	if [ "$(1)" = "optimized-postgresql" ] || [ "$(1)" = "standard-postgresql" ]; then \
		echo "Warm-up phase: préchauffage de PostgreSQL ($(WARMUP_DURATION))..." ; \
		$(VEGETA) attack -rate=0 -workers=$(CONNECTIONS) -max-workers=$(CONNECTIONS) -duration=$(WARMUP_DURATION) -keepalive=true -targets=gen/targets_$(2)_set.txt | $(VEGETA) report ; \
	fi ; \
	echo "Executing SET benchmark with Vegeta..." ; \
	$(VEGETA) attack -rate=0 -workers=$(CONNECTIONS) -max-workers=$(CONNECTIONS) -duration=$(DURATION) -keepalive=true -targets=gen/targets_$(2)_set.txt | $(VEGETA) report ; \
	echo "Executing GET benchmark with Vegeta..." ; \
	$(VEGETA) attack -rate=0 -workers=$(CONNECTIONS) -max-workers=$(CONNECTIONS) -duration=$(DURATION) -keepalive=true -targets=gen/targets_$(2)_get.txt | $(VEGETA) report ; \
	echo "Stopping server (PID $$pid)..." ; \
	kill $$pid 2>/dev/null || true ; \
	wait $$pid 2>/dev/null || true ; \
	echo "Finished $(1) benchmark." ; \
	pids=$$(lsof -t -i:$(PORT) 2>/dev/null) ; if [ -n "$$pids" ]; then kill -9 $$pids 2>/dev/null || true ; fi
endef
