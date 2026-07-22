# PostgreSQL as a High-Performance Cache: Comparative Benchmark

A technical, production-grade benchmarking suite evaluating **PostgreSQL** as a high-throughput, low-latency Key-Value cache against **Valkey** (Redis open-source fork) and an in-memory Go cache engine (**Otter**).

---

## 💡 Overview & Objective

In modern cloud architectures, introducing a dedicated key-value store (such as Redis or Valkey) is the standard pattern for application caching. While effective, this approach introduces operational complexity: managing additional cluster nodes, handling cross-network latency, maintaining data synchronization, and managing additional infrastructure costs.

This project empirically investigates a critical architectural question:

> **Can a properly tuned PostgreSQL instance serve as an enterprise-grade, high-throughput key-value cache while preserving data locality and simplifying infrastructure?**

To answer this, we measure throughput (MB/s), request execution rates (RPS), and latency distribution under sustained concurrency across four distinct caching paradigms over an endurance testing protocol.

---

## 🏗️ Architecture & Evaluated Engines

The benchmark runner executes an ultra-low-allocation HTTP service written in Go (`fasthttp`), exposing standardized GET/SET endpoints across four storage backends:

```
                      +----------------------------------+
                      |       Vegeta Load Generator      |
                      +----------------------------------+
                                       |
                                       v
                      +----------------------------------+
                      |   Go fasthttp Engine (Port 8080) |
                      |    (Zero-Alloc Buffer Recycling) |
                      +----------------------------------+
                                       |
     +-------------------+-------------+-------------+-------------------+
     |                   |                           |                   |
     v                   v                           v                   v
+----------+   +-------------------+   +--------------------+   +--------------------+
|  Otter   |   |   Valkey 9.1      |   | Standard Postgres  |   | Optimized Postgres |
| (Memory) |   | (Protobuf VTProto)|   | (Relational / UUID)|   | (UNLOGGED / VTProto|
+----------+   +-------------------+   +--------------------+   +--------------------+
```

### Evaluated Configurations

1. **In-Memory (`Otter`)**
   - **Mechanism:** Native Go in-memory cache leveraging `github.com/maypok86/otter/v2` with automated lock-free TTL expiration.
   - **Role:** Serves as the theoretical performance ceiling (zero network, zero serialization overhead).

2. **Valkey Key-Value Store**
   - **Mechanism:** Valkey 9.1 standalone instance connected via a tuned TCP connection pool (2,500 connections).
   - **Payload Format:** Protobuf binary wire format compiled via `planetscale/vtprotobuf` (`MarshalVT` / `UnmarshalVT`).
   - **Indexing:** 26-character Base32 Crockford ULIDs backed by `github.com/oklog/ulid/v2`.

3. **Standard PostgreSQL (`Relational / Flat`)**
   - **Mechanism:** Standard relational table (`users_standard`) with individual columns per field.
   - **Queries:** Prepared statements executed at the connection layer (`pgxpool`) to eliminate SQL parsing overhead.
   - **Indexing:** Binary UUID primary key (`UUID` / 16 bytes).

4. **Optimized PostgreSQL (`UNLOGGED / Protobuf VTProto`)**
   - **Mechanism:** Dedicated key-value cache architecture leveraging advanced PostgreSQL internals designed for transient workloads.
   - **Payload Format:** Protobuf VTProto zero-allocation byte payload stored in a bytea column.

---

## ⚡ Technical Core & System Optimizations

### 1. Protobuf VTProto (Zero-Allocation Serialization)

Instead of Go's reflection-heavy standard `proto.Marshal` or `json.Marshal`, all structured payloads are serialized using **PlanetScale's `vtprotobuf`** generator:

- **Zero Heap Allocations:** Encodes (`MarshalVT`) and decodes (`UnmarshalVT`) binary payloads without runtime reflection or dynamic struct allocations.
- **Wire Format Compatibility:** Fully compatible with standard Protobuf v3 specifications.

### 2. PostgreSQL Engine Tuning Strategies

The `Optimized PostgreSQL` configuration incorporates several database kernel optimizations:

- **`UNLOGGED` Tables:** Disables Write-Ahead Logging (WAL). Eliminates disk I/O bottlenecks during cache mutations (`SET`), enabling near-in-memory write speeds.
- **HOT (Heap-Only Tuple) Optimization (`fillfactor = 70`):** Reserves 30% page space on table blocks to allow in-place tuple updates. Reduces B-Tree index maintenance and prevents index bloat during frequent row overwrites.
- **Non-Blocking Asynchronous Purge (`FOR UPDATE SKIP LOCKED`):** Expired cache items are purged in background batches via PL/pgSQL (`purge_expired_cache_keys()`) using non-blocking row locks. Ensures active `GET` requests never block on garbage collection.
- **16-Byte Compact Binary Keys:** Keys are stored as native 16-byte binary UUIDs/ULIDs, maintaining a minimal B-Tree index footprint that fits completely inside PostgreSQL `shared_buffers`.

### 3. Application Layer & Runtime Optimization

- **`fasthttp` Core:** Replaces standard `net/http` with high-performance byte-slice parsing.
- **Struct Pooling:** Recycles `model.UserData` domain entities via `sync.Pool` during deserialization, eliminating Garbage Collector overhead under heavy GET loads.
- **Standardized Identifier Libraries:** Leverages `github.com/oklog/ulid/v2` for monotonic ULID generation and `github.com/google/uuid` for standard UUID operations.

---

## 📋 Methodology & Testing Protocol

To ensure reproducible and un-biased metrics, tests adhere to a strict isolation protocol:

1. **Hardware & Process Isolation:**
   - Go Server: Bound to 4 CPU cores (`GOMAXPROCS=4`) and 10GiB memory limit (`GOMEMLIMIT=10GiB`).
   - PostgreSQL: Constrained via `systemd` cgroups (`CPUQuota=200%`, `MemoryMax=4G`).
   - Valkey: Constrained via `systemd` cgroups (`CPUQuota=200%`, `MemoryMax=4G`). Even if Valkey is mono-threaded, it benefits from dedicated resources to handle the I/O load.

2. **OS Network Socket Tuning:**
   - Automated TCP socket recycling (`net.ipv4.tcp_tw_reuse=1`) and ephemeral port range expansion (`1024-65535`) to prevent `TIME_WAIT` socket exhaustion under heavy load.

3. **Execution Sequence:**
   - **Dataset Population:** Pre-populates 100,000 unique keys (`gen/keys.txt`).
   - **Warmup Phase:** 30-second target warmup to ensure cache pages and buffer pools are hot before measurement starts.
   - **Endurance Phase:** 10-minute continuous sustained load test using Vegeta (`-rate=0`, `-workers=250`).

---

## 🛠️ Prerequisites & Setup

### Requirements

- **Go** 1.26+
- **PostgreSQL** 18+
- **Valkey** 9.1+ (or Redis)
- **Vegeta** (`go install github.com/tsenart/vegeta/v12@latest`)
- **protoc** & `protoc-gen-go-vtproto` (optional, for regenerating protobuf code)

### Kernel Tuning

Apply Linux TCP socket parameter adjustments prior to running benchmarks:

```bash
make tune-os
```

---

## 🚀 Running the Benchmarks

### 1. Build & Generate Test Data

```bash
# Compile server binary (bin/server)
make build

# Generate 100,000 ULID test keys in gen/keys.txt
make gen-targets

# (Optional) Regenerate Protobuf VTProto code
make proto-gen
```

### 2. Execute Benchmark Suites

The Makefile provides three execution durations:

```bash
# Quick validation smoke test (5 seconds per engine)
make quick

# Medium benchmark run (300 seconds per engine)
make medium

# Official endurance test (600 seconds / 10 minutes per engine)
make long
```

### 3. Manual Server Execution

To run a specific engine manually for profiling or debugging:

```bash
# Launch server for a specific storage engine
./bin/server -engine optimized-postgresql -log-level production

# Execute Vegeta load test in a separate terminal
(while cat gen/keys.txt; do :; done) | \
  sed 's|^|GET http://127.0.0.1:8080/postgres/get?id=|' | \
  vegeta attack -lazy -rate=0 -workers=250 -duration=60s | \
  vegeta report
```

#### Exposed HTTP Endpoints (`:8080`)

| Engine                 | GET Endpoint                 | POST Endpoint                 |
| :--------------------- | :--------------------------- | :---------------------------- |
| **In-Memory**          | `GET /memory/get?id=<KEY>`   | `POST /memory/set?id=<KEY>`   |
| **Valkey**             | `GET /valkey/get?id=<KEY>`   | `POST /valkey/set?id=<KEY>`   |
| **Standard Postgres**  | `GET /postgres/get?id=<KEY>` | `POST /postgres/set?id=<KEY>` |
| **Optimized Postgres** | `GET /postgres/get?id=<KEY>` | `POST /postgres/set?id=<KEY>` |

---

## 📊 Benchmark Results

> ℹ️ _Official endurance results (10-minute continuous load suite) will be added here upon completion of the benchmark execution._

---

## 📄 License

This repository is distributed under the MIT License. See `LICENSE` for details.
