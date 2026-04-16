# PostgreSQL CDC Benchmark Results

Benchmarks for the `postgres_cdc` input using logical replication (snapshot mode).

See [`internal/impl/postgresql/bench/`](../../internal/impl/postgresql/bench/) for the benchmark configs and run instructions.

## Snapshot — Small Rows (cart table), CPU & Batch Size Scaling

Full snapshot of the `public.cart` table: 10,000,000 rows, ~600 B per row. Varying `GOMAXPROCS` and `batching.count` to produce a throughput matrix.

**Environment:** Intel Core i7-10850H @ 2.70GHz, 32 GB RAM, WSL2 (Linux 6.6.87.2), x86_64, PostgreSQL 16 running in Docker (localhost)

**Dataset:** 10,000,000 rows × ~600 B

### msg/sec

| GOMAXPROCS  | batch=1000 | batch=5000 | batch=10000 |
|-------------|------------|------------|-------------|
| 1           |    134,287 |    130,555 |     129,603 |
| 2           |    212,852 |    218,055 |     214,555 |
| 4           |    276,259 |    296,138 |     264,454 |
| 8           |    300,760 |    318,660 |     284,733 |
| (unbounded) |    211,111 |            |             |

### MB/sec

| GOMAXPROCS  | batch=1000 | batch=5000 | batch=10000 |
|-------------|------------|------------|-------------|
| 1           |         81 |         78 |          78 |
| 2           |        128 |        131 |         129 |
| 4           |        166 |        178 |         159 |
| 8           |        181 |        192 |         171 |
| (unbounded) |        127 |            |             |

**Observations:**

- **Core scaling:** throughput scales well up to 4 cores then plateaus — 1→2: ~1.58x, 2→4: ~1.30x, 4→8: ~1.09x across all batch sizes.
- **Batch size sweet spot:** batch=5000 consistently outperforms both batch=1000 and batch=10000. At 8 cores: 318K (batch=5000) vs 300K (batch=1000) vs 284K (batch=10000).
- **Batch=10000 regresses at higher core counts:** larger batches increase memory pressure and pipeline stall time waiting to fill a batch, which outweighs the reduced per-batch overhead.
- **At 1 core, batch size has no effect** (~130K msg/sec across all three), confirming the single-core bottleneck is not batch assembly but connector read throughput.

---

## Snapshot — Large Rows (users table), CPU & Batch Size Scaling

Full snapshot of the `public.users` table: 150,000 rows, ~625 KB per row. Varying `GOMAXPROCS` and `batching.count` to measure throughput on large I/O bound messages.

**Environment:** Intel Core i7-10850H @ 2.70GHz, 32 GB RAM, WSL2 (Linux 6.6.87.2), x86_64, PostgreSQL 16 running in Docker (localhost)

**Dataset:** 150,000 rows × ~625 KB

### msg/sec

| GOMAXPROCS  | batch=1000 | batch=5000 | batch=10000 |
|-------------|------------|------------|-------------|
| 1           |        883 |        843 |         N/A |
| 2           |      1,166 |      1,134 |       1,024 |
| 4           |      1,145 |        N/A |         N/A |
| 8           |      1,145 |        N/A |         N/A |

### MB/sec

| GOMAXPROCS  | batch=1000 | batch=5000 | batch=10000 |
|-------------|------------|------------|-------------|
| 1           |        580 |        554 |         N/A |
| 2           |        766 |        745 |         673 |
| 4           |        752 |        N/A |         N/A |
| 8           |        752 |        N/A |         N/A |

**Observations:** Throughput plateaus at 2 cores (1,166 msg/sec, 766 MB/sec) and is completely flat from 4→8 cores. This confirms the users dataset is I/O bound — additional cores provide no benefit. Testing further batch sizes is unlikely to change this conclusion. Contrast with cart where throughput scaled to 318K msg/sec at 8 cores.

---

## Kafka → PostgreSQL: Kafka Connect (JDBC Sink) vs Redpanda Connect

Comparison of throughput writing from Kafka into PostgreSQL. Both connectors read from the same 16-partition `bench-events` topic and write to a `bench_events` table.

**Environment:** Intel Core i7-10850H @ 2.70GHz, 32 GB RAM, WSL2 (Linux 6.6.87.2), x86_64, Kafka + PostgreSQL 16 running in Docker (localhost)

**Dataset:** 10,000,000 rows × ~200 B (synthetic events: id, category, value, ts)

See [`internal/impl/postgresql/bench/kafka-connector/`](../../internal/impl/postgresql/bench/kafka-connector/) for the Kafka Connect benchmark config and run instructions.

### Results

| Connector | Tasks / Workers | Total Messages | Elapsed | Avg Throughput |
|---|---|---|---|---|
| Kafka Connect (JDBC Sink) | 16 tasks | 10,000,000 | 55s | 181,818 msg/s |
| Redpanda Connect | — | — | — | — |

### Notes

- **Kafka Connect** used `confluentinc/kafka-connect-jdbc:10.7.14` with 16 sink tasks, batch size 3,000, `insert` mode, and JSON converter with embedded schema envelope (no Schema Registry).
- Redpanda Connect results pending.
