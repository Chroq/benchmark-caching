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

The benchmark runner executes an ultra-low-allocation HTTP service written in Go (`fasthttp`), exposing standardized GET/SET endpoints across five storage backends:

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
     +------------+----------+---------+---------+-------------------+
     |            |          |                   |                   |
     v            v          v                   v                   v
+----------+ +---------+ +--------------------+ +--------------------+ +--------------------+
|  Otter   | | Valkey  | | Standard Postgres  | | Optimized Postgres | |   Postgres TSID    |
| (Memory) | |  (9.1)  | |(Relational/UUIDv7) | |(UNLOGGED/VTProto)  | | (64-bit TSID Key)  |
+----------+ +---------+ +--------------------+ +--------------------+ +--------------------+
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
   - **Mechanism:** Standard relational table (`users_standard`) with individual columns per field, pre-populated with **10,000,000 background rows** (~1.3 GB of data) to evaluate query latency against a production-scale relational table.
   - **Queries:** Prepared statements executed at the connection layer (`pgxpool`) to eliminate SQL parsing overhead.
   - **Indexing:** Binary UUID primary key (`UUID` / 16 bytes).

4. **Optimized PostgreSQL (`UNLOGGED / Protobuf VTProto`)**
   - **Mechanism:** Dedicated key-value cache architecture (`cache_optimized` unlogged table). Designed for transient workloads, it operates as a dedicated cache store holding the active working set (100,000 seeded keys).
   - **Payload Format:** Protobuf VTProto zero-allocation byte payload stored in a compact `bytea` column.
   - **Kernel Optimizations:** Disables Write-Ahead Logging (`UNLOGGED`), reserves page space for HOT updates (`fillfactor = 70`), and executes non-blocking background cleanup (`FOR UPDATE SKIP LOCKED`).

5. **PostgreSQL TSID (`Relational / 64-bit TSID`)**
   - **Mechanism:** Relational table schema (`users_tsid`) leveraging 64-bit Time-Sorted Identifiers (TSID) stored as native 8-byte `bigint` primary keys, pre-populated with **10,000,000 background rows** (~1.3 GB of data) for compact B-Tree index traversal.

---

## ⚡ Technical Core & System Optimizations

### 1. Protobuf VTProto (Zero-Allocation Serialization)

Instead of Go's reflection-heavy standard `proto.Marshal` or `json.Marshal`, all structured payloads are serialized using **PlanetScale's `vtprotobuf`** generator:

- **Zero Heap Allocations:** Encodes (`MarshalVT`) and decodes (`UnmarshalVT`) binary payloads without runtime reflection or dynamic struct allocations.
- **Wire Format Compatibility:** Fully compatible with standard Protobuf v3 specifications.

### 2. PostgreSQL Engine Tuning Strategies (`postgresql.conf`)

PostgreSQL is engineered by default for strict ACID data durability. To maximize throughput and minimize latency for caching workloads, RAM allocations, Write-Ahead Logging (WAL), and page management must be tuned (`postgresql.conf` or `ALTER SYSTEM SET ...`):

#### A. Memory Allocation (RAM Optimization)

- **`shared_buffers = 25%` of Total System RAM** _(e.g., 1GB on a 4GB system, 4GB on 16GB RAM)_: Primary shared memory cache where PostgreSQL buffers table pages and B-Tree index blocks read from disk.
- **`work_mem = 16MB` to `64MB`**: Memory allocated for sorting operations (`ORDER BY`, `DISTINCT`, `JOIN`). Allocated per query operation per connection; tuned to prevent Linux Kernel OOM invocation under high concurrent connection pools.
- **`effective_cache_size = 50%` to `75%` of Total System RAM**: Estimate provided to the query planner representing available RAM (PostgreSQL shared buffers + Linux OS page cache), encouraging high-performance B-Tree index scans.
- **`maintenance_work_mem = 256MB` to `1GB`**: Accelerates index creation (`CREATE INDEX`), table maintenance (`VACUUM`), and bulk foreign key validations.

#### B. Checkpoints & WAL Tuning (High-Throughput SET Performance)

- **`max_wal_size = 4GB` to `16GB`** _(increased from default 1GB)_: Expands WAL log capacity before triggering forced checkpoints, eliminating disk I/O contention during sustained write spikes.
- **`checkpoint_completion_target = 0.9`**: Spreads checkpoint disk writes over 90% of the checkpoint duration window, preventing latency spikes.
- **`wal_buffers = 16MB`**: Memory buffer allocation for unwritten WAL data prior to disk flushing.

#### C. Transient Cache Architecture (`UNLOGGED` & HOT Updates)

- **`UNLOGGED` Tables:** Disables Write-Ahead Logging (WAL) for transient cache items, achieving near-in-memory write speeds by eliminating disk I/O synchronization.
- **HOT (Heap-Only Tuple) Optimization (`fillfactor = 70`):** Reserves 30% page space on table blocks to allow in-place tuple updates, eliminating B-Tree index rewrite overhead during frequent key overwrites.
- **Non-Blocking Asynchronous Purge (`FOR UPDATE SKIP LOCKED`):** Expired cache items are purged in background batches via PL/pgSQL using non-blocking row locks, preventing `GET` requests from blocking during garbage collection.
- **Compact Keys:** 16-byte binary UUID v7 / ULID and 8-byte TSID keys keep index node sizes minimal, ensuring B-Tree structures fit inside `shared_buffers`.

---

### 3. Valkey Engine Tuning Strategies (`valkey.conf`)

Valkey/Redis executes on a single primary event-loop thread for core data manipulations. Performance bottlenecks stem from single-core CPU execution and network I/O bandwidth. The following configurations optimize Valkey as an in-memory cache:

#### A. Memory Management & Eviction Policy

- **`maxmemory = 75%` of Server RAM** _(e.g., `maxmemory 3gb` on a 4GB server)_: Sets a strict memory limit to prevent Linux Kernel OOM killer termination.
- **`maxmemory-policy = allkeys-lru` or `volatile-lru`**: Automatically evicts Least Recently Used (LRU) keys when memory limit is reached to accommodate new cache insertions.

#### B. Network I/O Threading (`io-threads`)

- **`io-threads = 2` or `4`**: Offloads RESP protocol parsing and socket write operations to secondary worker threads (e.g., 2-3 threads on a 4-vCPU system), freeing the primary thread exclusively for in-memory key execution.
- **`io-threads-do-reads = yes`**: Enables multi-threaded network reading and payload parsing for incoming request sockets.

#### C. Disk Persistence Offloading (Pure In-Memory Cache)

When operating exclusively as a transient application cache:

- **`save ""`**: Disables background RDB disk snapshots.
- **`appendonly no`**: Disables Append-Only File (AOF) logging to save CPU cycles and eliminate disk I/O latency.

---

### 4. Application Layer & Runtime Optimization

- **`fasthttp` Core:** Replaces standard `net/http` with high-performance byte-slice parsing and connection buffer recycling.
- **Struct Pooling:** Recycles `model.UserData` domain entities via `sync.Pool` during deserialization, eliminating Garbage Collector overhead under heavy GET loads.
- **Standardized Identifier Libraries:** Leverages `github.com/google/uuid` for monotonic UUID v7 generation, `github.com/oklog/ulid/v2` for 26-character Base32 ULIDs, and 64-bit TSIDs (`bigint`).


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

### Kernel & OS Tuning (`make tune-os`)

> [!WARNING]
> **System Configuration Modification Warning:**
> Executing `make tune-os` requires elevated privileges (`sudo`) and **permanently modifies system-level kernel parameters** and service cgroups on the host operating system:
> - **Kernel `sysctl` Network Tuning:** Modifies TCP socket recycling (`net.ipv4.tcp_tw_reuse=1`), expands local ephemeral port ranges (`1024-65535`), increases connection backlogs (`net.core.somaxconn=65535`, `tcp_max_syn_backlog=65535`), and reduces FIN timeout (`tcp_fin_timeout=15`).
> - **Systemd Service Cgroup Limits:** Sets strict CPU quotas (`CPUQuota=200%`) and memory limits (`MemoryMax=4G`) directly on system services (`postgresql` and `valkey`).

```bash
# Apply Linux kernel socket and systemd cgroup tuning parameters
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
| **Postgres TSID**      | `GET /postgres/get?id=<KEY>` | `POST /postgres/set?id=<KEY>` |

---

## 📊 Benchmark Results

> [!IMPORTANT]
> **Dataset Footprint & Benchmark Isolation Context:**
>
> - **Relational Baselines (`standard-postgresql` & `postgres-tsid`):** Evaluated against relational tables pre-populated with **10,000,000 background rows (~1.3 GB of data)** to measure B-Tree index traversal, memory page cache behavior, and lookup latency against a large enterprise relational table footprint.
> - **Dedicated Cache Engines (`memory`, `valkey`, `optimized-postgresql`):** Evaluated as dedicated key-value cache stores initialized with the active **100,000 key working set**.

### 1. GET Read Endurance Benchmark (10 Minutes / 600s Continuous Load)

_Evaluates read throughput and latency distribution under heavy concurrent load (250 workers)._

| Engine                 | Storage Paradigm            |    Dataset Size    | Total Requests |   Throughput (RPS)   | Mean Latency | p50 Latency | p90 Latency | p95 Latency | p99 Latency | Max Latency | Success |
| :--------------------- | :-------------------------- | :----------------: | :------------: | :------------------: | :----------: | :---------: | :---------: | :---------: | :---------: | :---------: | :-----: |
| **In-Memory (Otter)**  | Native Go Cache             |     100k keys      |   63,995,364   | **106,658.98 req/s** |   1.959ms    |   1.482ms   |   4.415ms   |   5.287ms   |   7.687ms   |  26.758ms   | 100.00% |
| **Valkey 9.1**         | Standalone KV Store         |     100k keys      |   46,273,423   | **77,122.39 req/s**  |   2.955ms    |   2.800ms   |   4.749ms   |   5.396ms   |   6.782ms   |  30.598ms   | 100.00% |
| **Postgres TSID**      | Relational DB (64-bit TSID) | 10M rows (~1.3 GB) |   32,395,151   | **53,991.93 req/s**  |   4.321ms    |   4.202ms   |   6.242ms   |   7.096ms   |   9.236ms   |  33.767ms   | 100.00% |
| **Optimized Postgres** | UNLOGGED Cache Table        |     100k keys      |   32,249,699   | **53,749.43 req/s**  |   4.338ms    |   4.226ms   |   6.274ms   |   7.133ms   |   9.237ms   |  47.219ms   | 100.00% |
| **Standard Postgres**  | Relational DB (UUID v7)     | 10M rows (~1.3 GB) |   31,991,527   | **53,319.26 req/s**  |   4.382ms    |   4.317ms   |   6.226ms   |   6.974ms   |   8.585ms   |  39.832ms   | 100.00% |

### 2. SET Write Benchmark (Seeding 100,000 Keys)

_Measures key insertion throughput and write latency required to populate 100k keys._

| Engine                 | Total Requests |     Rate (RPS)      | Duration | Mean Latency | p50 Latency | p90 Latency | p95 Latency | p99 Latency | Max Latency | Success |
| :--------------------- | :------------: | :-----------------: | :------: | :----------: | :---------: | :---------: | :---------: | :---------: | :---------: | :-----: |
| **In-Memory (Otter)**  |    100,011     | **94,029.10 req/s** |  1.065s  |   2.124ms    |   1.667ms   |   4.617ms   |   5.627ms   |   8.420ms   |  22.864ms   | 99.99%  |
| **Valkey 9.1**         |    100,122     | **68,980.18 req/s** |  1.452s  |   3.289ms    |   3.025ms   |   5.082ms   |   5.822ms   |   7.840ms   |  52.463ms   | 99.88%  |
| **Optimized Postgres** |    100,013     | **29,405.72 req/s** |  3.406s  |   8.415ms    |   6.384ms   |   8.152ms   |  30.372ms   |  32.468ms   |  35.232ms   | 99.99%  |
| **Standard Postgres**  |    100,001     | **2,754.07 req/s**  | 36.397s  |   90.831ms   |  89.224ms   |  93.973ms   |  95.787ms   |  173.848ms  |  727.267ms  | 100.00% |
| **Postgres TSID**      |    100,003     | **2,672.79 req/s**  | 37.506s  |   93.600ms   |  89.796ms   |  95.459ms   |  101.944ms  |  253.804ms  |  938.118ms  | 100.00% |

### 💡 Key Takeaways & Performance Insights

1. **Read Scale Efficiency on a 10M Row (~1.3 GB) Dataset:**
   - Even when querying relational tables scaled to **10,000,000 rows (~1.3 GB)** (`standard-postgresql` and `postgres-tsid`), PostgreSQL maintains **~53,300 - 54,000 req/s** with a p50 latency of **4.2ms**.
   - PostgreSQL achieves **70% of dedicated Valkey/Redis read throughput** (54k vs 77k RPS) under heavy concurrent load, demonstrating that PostgreSQL B-Tree index traversal remains fast even at scale.

2. **10.6x Write Acceleration via UNLOGGED Tables & Protobuf:**
   - **Optimized PostgreSQL** (`UNLOGGED` + `fillfactor=70` + Protobuf VTProto bytea payload) achieves **29,405.72 req/s** on SET operations (3.4s duration for 100k keys), compared to **2,754.07 req/s** for Standard PostgreSQL (36.4s duration).
   - Eliminating WAL disk synchronization overhead yields a **10.6x performance gain** for cache write operations.

<details>
<summary><b>🔍 Click to view Raw Cleaned Benchmark Output Log</b></summary>

```text
========================================================
CLEANED BENCHMARK RESULTS
========================================================

========================================================
ENGINE: memory
========================================================
Successfully generated 100000 raw UUID v7 keys in gen/keys.txt

--- Executing SET benchmark (Seeding 100k keys)... ---
Requests      [total, rate, throughput]         100011, 94029.10, 93917.43
Duration      [total, attack, wait]             1.065s, 1.064s, 1.148ms
Latencies     [min, mean, 50, 90, 95, 99, max]  120ns, 2.124ms, 1.667ms, 4.617ms, 5.627ms, 8.42ms, 22.864ms
Bytes In      [total, mean]                     0, 0.00
Bytes Out     [total, mean]                     0, 0.00
Success       [ratio]                           99.99%
Status Codes  [code:count]                      0:11  200:100000

--- Executing GET benchmark with Vegeta (600s)... ---
Requests      [total, rate, throughput]         63995364, 106658.98, 106658.43
Duration      [total, attack, wait]             10m0s, 10m0s, 3.07ms
Latencies     [min, mean, 50, 90, 95, 99, max]  31.86µs, 1.959ms, 1.482ms, 4.415ms, 5.287ms, 7.687ms, 26.758ms
Bytes In      [total, mean]                     0, 0.00
Bytes Out     [total, mean]                     0, 0.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:63995364

========================================================
ENGINE: valkey
========================================================
Successfully generated 100000 raw UUID v7 keys in gen/keys.txt

--- Executing SET benchmark (Seeding 100k keys)... ---
Requests      [total, rate, throughput]         100122, 68980.18, 68871.16
Duration      [total, attack, wait]             1.452s, 1.451s, 526.102µs
Latencies     [min, mean, 50, 90, 95, 99, max]  110ns, 3.289ms, 3.025ms, 5.082ms, 5.822ms, 7.84ms, 52.463ms
Bytes In      [total, mean]                     0, 0.00
Bytes Out     [total, mean]                     0, 0.00
Success       [ratio]                           99.88%
Status Codes  [code:count]                      0:122  200:100000

--- Executing GET benchmark with Vegeta (600s)... ---
Requests      [total, rate, throughput]         46273423, 77122.39, 77122.12
Duration      [total, attack, wait]             10m0s, 10m0s, 2.077ms
Latencies     [min, mean, 50, 90, 95, 99, max]  69.701µs, 2.955ms, 2.8ms, 4.749ms, 5.396ms, 6.782ms, 30.598ms
Bytes In      [total, mean]                     0, 0.00
Bytes Out     [total, mean]                     0, 0.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:46273423

========================================================
ENGINE: standard-postgresql
========================================================
Successfully generated 100000 raw UUID v7 keys in gen/keys.txt

--- Executing SET benchmark (Seeding 100k keys)... ---
Requests      [total, rate, throughput]         100001, 2754.07, 2747.51
Duration      [total, attack, wait]             36.397s, 36.31s, 86.251ms
Latencies     [min, mean, 50, 90, 95, 99, max]  882ns, 90.831ms, 89.224ms, 93.973ms, 95.787ms, 173.848ms, 727.267ms
Bytes In      [total, mean]                     0, 0.00
Bytes Out     [total, mean]                     0, 0.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      0:1  200:100000

--- Executing GET benchmark with Vegeta (600s)... ---
Requests      [total, rate, throughput]         31991527, 53319.26, 53318.95
Duration      [total, attack, wait]             10m0s, 10m0s, 3.43ms
Latencies     [min, mean, 50, 90, 95, 99, max]  102.814µs, 4.382ms, 4.317ms, 6.226ms, 6.974ms, 8.585ms, 39.832ms
Bytes In      [total, mean]                     0, 0.00
Bytes Out     [total, mean]                     0, 0.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:31991527

========================================================
ENGINE: optimized-postgresql
========================================================
Successfully generated 100000 raw UUID v7 keys in gen/keys.txt

--- Executing SET benchmark (Seeding 100k keys)... ---
Requests      [total, rate, throughput]         100013, 29405.72, 29361.66
Duration      [total, attack, wait]             3.406s, 3.401s, 4.661ms
Latencies     [min, mean, 50, 90, 95, 99, max]  340ns, 8.415ms, 6.384ms, 8.152ms, 30.372ms, 32.468ms, 35.232ms
Bytes In      [total, mean]                     0, 0.00
Bytes Out     [total, mean]                     0, 0.00
Success       [ratio]                           99.99%
Status Codes  [code:count]                      0:13  200:100000

--- Executing GET benchmark with Vegeta (600s)... ---
Requests      [total, rate, throughput]         32249699, 53749.43, 53749.21
Duration      [total, attack, wait]             10m0s, 10m0s, 2.444ms
Latencies     [min, mean, 50, 90, 95, 99, max]  81.534µs, 4.338ms, 4.226ms, 6.274ms, 7.133ms, 9.237ms, 47.219ms
Bytes In      [total, mean]                     0, 0.00
Bytes Out     [total, mean]                     0, 0.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:32249699

========================================================
ENGINE: postgres-tsid
========================================================
Successfully generated 100000 raw TSID 64-bit keys in gen/keys.txt

--- Executing SET benchmark (Seeding 100k keys)... ---
Requests      [total, rate, throughput]         100003, 2672.79, 2666.21
Duration      [total, attack, wait]             37.506s, 37.415s, 91.308ms
Latencies     [min, mean, 50, 90, 95, 99, max]  401ns, 93.6ms, 89.796ms, 95.459ms, 101.944ms, 253.804ms, 938.118ms
Bytes In      [total, mean]                     0, 0.00
Bytes Out     [total, mean]                     0, 0.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      0:3  200:100000

--- Executing GET benchmark with Vegeta (600s)... ---
Requests      [total, rate, throughput]         32395151, 53991.93, 53991.62
Duration      [total, attack, wait]             10m0s, 10m0s, 3.404ms
Latencies     [min, mean, 50, 90, 95, 99, max]  105.509µs, 4.321ms, 4.202ms, 6.242ms, 7.096ms, 9.236ms, 33.767ms
Bytes In      [total, mean]                     0, 0.00
Bytes Out     [total, mean]                     0, 0.00
Success       [ratio]                           100.00%
Status Codes  [code:count]                      200:32395151
```

</details>

---

## ⚖️ Comprehensive Benchmark Considerations & Architectural Trade-Offs

When choosing between a dedicated in-memory key-value cache (Valkey/Redis) and leveraging PostgreSQL as a high-performance cache store, architects must evaluate six critical considerations:

### 1. Data Locality & Infrastructure Complexity
* **Single-Database Architecture (PostgreSQL Only):**
  * **Pros:** Simplifies system topology, eliminates cache invalidation race conditions, reduces Cloud infrastructure cost (no extra Redis nodes/clusters), unifies backup and point-in-time recovery (PITR).
  * **Cons:** Shares CPU/RAM resources between relational queries and caching workloads.
* **Dual-Database Architecture (PostgreSQL + Valkey/Redis):**
  * **Pros:** Isolate cache workloads to dedicated memory-optimized nodes; sub-millisecond latencies under ultra-high RPS (>75k+ req/s).
  * **Cons:** Introduces cross-network RPC latency, cache synchronization complexity, dual-write consistency hazards, and additional node management overhead.

### 2. Memory Footprint & Page Eviction Behavior
* **RAM-Bound Stores (Valkey / Otter):**
  * Store all keys and values strictly in volatile RAM. When capacity (`maxmemory`) is exceeded, data must be evicted using policies like `allkeys-lru` or writes will fail.
* **Disk-Backed RAM Cache (PostgreSQL):**
  * PostgreSQL uses a hybrid memory model: active pages and index nodes sit in `shared_buffers` and Linux OS page cache. If RAM is exceeded, cold pages are transparently paged to disk instead of triggering memory eviction or OOM crashes.

### 3. Durability vs. Write Speed Spectrum
* **ACID Logged Storage (`standard-postgresql` / `postgres-tsid`):**
  * Guarantees 100% crash safety via Write-Ahead Logging (WAL). Every mutation (`SET`) waits for synchronous disk journal flushes, capping write throughput at **~2,750 req/s**.
* **Transient `UNLOGGED` Storage (`optimized-postgresql`):**
  * Bypasses WAL logging entirely, raising write throughput by **10.6x to 29,400 req/s**. If the database node crashes, `UNLOGGED` tables are truncated on restart—a perfect trade-off for cache tables where missing keys can be re-hydrated from source tables.

### 4. Primary Key Indexing Efficiency (UUID v7 vs 64-bit TSID vs ULID)
* **16-Byte Monotonic Identifiers (UUID v7 / ULID):**
  * Sequential timestamp prefixes ensure B-Tree insertions happen at the rightmost page edge, preventing page splitting and index bloat.
* **8-Byte Compact Identifiers (TSID `bigint`):**
  * Halves primary key storage (8 bytes vs 16 bytes). Smaller index nodes allow **2x more key entries per B-Tree page block**, maximizing CPU L1/L2 cache hit ratios and optimizing `shared_buffers` usage across 10,000,000+ rows.

### 5. Serialization Overhead (Protobuf VTProto vs. Relational Mapping)
* **Protobuf VTProto Binary Payloads:**
  * Serializes domain objects into zero-allocation byte arrays stored in a PostgreSQL `bytea` column. Eliminates multi-column SQL parsing and runtime Go reflection.
* **Relational Field Mapping:**
  * Maps individual struct fields to separate database columns. Provides SQL queryability (`WHERE age > 30`) at the cost of higher query parsing and tuple assembly overhead.

---

## 🎯 Architectural Recommendations & Typology Trade-Off Matrix


Based on empirical benchmark data, engineering complexity, energy efficiency, and operational SLAs, we formulate specific recommendations across four distinct caching paradigms (including the choice to eliminate caching altogether):

### 1. Typology Breakdown & Engineering Guidelines

#### 🟢 Typology A: Process-Local In-Memory Cache (`Otter`)
* **Core Profile:** Embedded Go memory cache executing in-process without network RPCs.
* **Performance:** Maximum throughput (**106.6k GET RPS / 94.0k SET RPS**) and lowest latency (**1.48ms p50**).
* **Energy & Hardware Sobriety:** **Maximum.** Zero network TCP socket allocations, zero protocol serialization, and zero dedicated idle infrastructure.
* **Limitations:** Bound to single-node application memory; cannot be shared across horizontally scaled microservice replicas without external synchronization.
* **Recommendation:** Ideal for **L1 micro-caching**, immutable metadata, static application configurations, or single-process hot-path lookups.

#### 🟢 Typology B: Distributed In-Memory Key-Value Store (`Valkey 9.1` / Redis)
* **Core Profile:** Dedicated standalone/clustered key-value cache running on isolated memory-optimized instances.
* **Performance:** Excellent throughput (**77.1k GET RPS / 69.0k SET RPS**) and low latency (**2.80ms p50**).
* **Energy & Hardware Sobriety:** **Medium.** Requires dedicated cloud servers/clusters running continuous event loops, introducing network serialization (RESP protocol) overhead and idle energy footprints.
* **Limitations:** Introduces operational complexity, cross-network RPC hops, cluster failover management, and risk of cache invalidation bugs (stale cache vs. primary DB).
* **Recommendation:** Essential for **multi-instance shared session stores**, distributed rate-limiting, real-time pub/sub, or applications requiring sustained >75,000+ GET req/s.

#### 🟢 Typology C: Native PostgreSQL UNLOGGED Cache Table (`optimized-postgresql`)
* **Core Profile:** Key-Value cache stored inside PostgreSQL using an `UNLOGGED` table (`fillfactor = 70`) and binary Protobuf VTProto payloads.
* **Performance:** Strong read throughput (**53.7k GET RPS**) and **10.6x faster write throughput** (**29.4k SET RPS**) compared to standard SQL transactions.
* **Energy & Hardware Sobriety:** **High.** Reuses existing database infrastructure, eliminating the energy footprint and cost of extra Redis/Valkey cluster instances and avoiding additional network hops when co-located.
* **Limitations:** `UNLOGGED` tables are truncated upon PostgreSQL hard crashes/reboots (ephemeral cache behavior). Missing keys must be re-hydrated from source tables.
* **Recommendation:** Ideal for teams seeking to **simplify infrastructure to a Single Database Stack** while requiring 10x-faster cache write mutations (29.4k RPS) alongside high read throughput (53.7k RPS).

#### 🟢 Typology D: Direct Query on Primary Relational Database / "No Cache Layer" (`standard-postgresql` / `postgres-tsid`)
* **Core Profile:** Querying the primary relational database tables directly, evaluated against **10,000,000 background rows (~1.3 GB)**.
* **Performance:** Exceptional read scale (**53.3k - 54.0k GET RPS @ 4.2ms p50 latency**). Write throughput is bound by WAL journal flushes (**~2,700 SET RPS**).
* **Energy & Hardware Sobriety:** **Maximum.** Eliminates data duplication, eliminates cache invalidation code, and avoids redundant dual-write CPU/network cycles.
* **Limitations:** Write throughput is capped by WAL disk I/O; heavy analytical writes can contend with read lookups.
* **Recommendation:** Recommended as the **default baseline architecture** for read-heavy applications (up to 50,000+ req/s) with write rates below ~2,500 req/s. Proves that a properly indexed 10M-row PostgreSQL table is often fast enough, avoiding premature optimization and extra infrastructure.

---

### 📌 Multi-Dimensional Decision Matrix

| Evaluation Dimension | Typology A: Local In-Memory (`Otter`) | Typology B: Distributed Store (`Valkey 9.1`) | Typology C: Postgres UNLOGGED Cache (`optimized-postgresql`) | Typology D: Direct DB / No Cache (`standard-postgresql` / `postgres-tsid`) |
| :--- | :---: | :---: | :---: | :---: |
| **GET Read Throughput (RPS)** | **106,658 req/s** 🚀 | **77,122 req/s** 🟢 | **53,749 req/s** 🟢 | **53,319 - 53,991 req/s** 🟢 |
| **SET Write Throughput (RPS)** | **94,029 req/s** 🚀 | **68,980 req/s** 🟢 | **29,405 req/s** 🟡 | **2,672 - 2,754 req/s** 🔴 |
| **p50 Read Latency** | **1.48 ms** | **2.80 ms** | **4.22 ms** | **4.20 - 4.31 ms** |
| **p99 Read Latency** | **7.69 ms** | **6.78 ms** | **9.24 ms** | **8.59 - 9.24 ms** |
| **Operational Complexity** | **Low (Infra) / Medium (App)** *(Requires app code to correlate cache state & sync with primary DB)* | **High** *(Cluster deployment, failover, Redis proxy)* | **Low** *(Reuses Postgres pool & schema)* | **Minimum** *(Single DB stack, single source of truth)* |
| **Energy & Hardware Efficiency** | **Maximum** *(Zero network I/O, zero idle server power)* | **Medium** *(Dedicated idle VMs/clusters, network serialization)* | **High** *(Reuses DB hardware, no extra network hops)* | **Maximum** *(Zero data duplication, zero dual-write CPU usage)* |
| **Consistency Hazards** | N/A *(Process-local)* | **High** *(Stale cache vs DB, invalidation race conditions)* | **Medium** *(Table-level TTL / background PL/pgSQL purge)* | **Zero Risk** *(100% ACID consistency guaranteed)* |
| **Durability / Crash Recovery** | Volatile *(Lost on process exit)* | Ephemeral / Configurable *(RDB/AOF)* | Ephemeral *(Truncated on DB hard restart)* | **100% ACID Guaranteed** *(Full WAL journal safety)* |
| **Memory Overflow Behavior** | App RAM bound | RAM bound *(LRU eviction)* | Hybrid *(RAM `shared_buffers` + transparent disk overflow)* | Hybrid *(RAM `shared_buffers` + transparent disk overflow)* |
| **Architectural Verdict** | **Best for L1 local cache & static config** | **Best for shared sessions & >75k+ RPS SLAs** | **Best for high-volume cache on single DB stack** | **Best default for <2.5k write RPS (Prevents over-engineering)** |


---

## 📄 License

This repository is distributed under the MIT License. See `LICENSE` for details.


