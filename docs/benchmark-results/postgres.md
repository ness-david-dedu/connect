# PostgreSQL CDC Benchmark Results

Benchmarks for the `postgres_cdc` input using logical replication (snapshot mode).

See [`internal/impl/postgresql/bench/`](../../internal/impl/postgresql/bench/) for the benchmark configs and run instructions.

## Pipeline Configuration

All results in this file use the following pipeline unless noted otherwise:

- **No processor** — raw connector throughput, output is `drop` with a `benchmark` processor
- **Batch size** — `batching.count: 1000`

Future sections will cover batch size scaling and transformation processors (passthrough mapping, light Bloblang).

---

## Snapshot — Large Rows (users table)

Full snapshot of the `public.users` table: 150,000 rows, ~625 KB per row (`about` field is `repeat(...)` producing ~500 KB of text).

### Run 1

**Environment:** Intel Core i7-10850H @ 2.70GHz, 32 GB RAM, WSL2 (Linux 6.6.87.2), x86_64, PostgreSQL 16 running in Docker (localhost)

**Dataset:** 150,000 rows × ~625 KB = ~93 GB uncompressed

**Configuration:**
- `stream_snapshot`: true
- `schema`: public
- `tables`: [users]
- `slot_name`: bench_slot
- `batching.count`: 1000

**Throughput:**

```
INFO rolling stats: 1000 msg/sec, 625 MB/sec    @service=redpanda-connect bytes/sec=6.2527229e+08 label="" msg/sec=1000 path=root.output.processors.0
INFO rolling stats: 2000 msg/sec, 1.3 GB/sec    @service=redpanda-connect bytes/sec=1.25054458e+09 label="" msg/sec=2000 path=root.output.processors.0
INFO rolling stats: 1000 msg/sec, 625 MB/sec    @service=redpanda-connect bytes/sec=6.2527229e+08 label="" msg/sec=1000 path=root.output.processors.0
INFO rolling stats: 1000 msg/sec, 625 MB/sec    @service=redpanda-connect bytes/sec=6.2527229e+08 label="" msg/sec=1000 path=root.output.processors.0
INFO rolling stats: 2000 msg/sec, 1.3 GB/sec    @service=redpanda-connect bytes/sec=1.25054458e+09 label="" msg/sec=2000 path=root.output.processors.0
INFO rolling stats: 1000 msg/sec, 625 MB/sec    @service=redpanda-connect bytes/sec=6.2527229e+08 label="" msg/sec=1000 path=root.output.processors.0
```

Steady state: **~1,000–2,000 msg/sec, 625 MB/sec – 1.3 GB/sec**, ~625 KB per message.

**Observations:** Throughput is dominated by message size — at ~625 KB per row the connector is largely I/O bound reading from the PostgreSQL snapshot. The batching count of 1,000 produces large in-flight batches which contribute to the throughput spikes reaching 1.3 GB/sec.

---

## Snapshot — Small Rows (cart table)

Full snapshot of the `public.cart` table: 10,000,000 rows, small payloads (~600 bytes per row). Run with no output backpressure (unbounded drop).

### Run 1

**Environment:** Intel Core i7-10850H @ 2.70GHz, 32 GB RAM, WSL2 (Linux 6.6.87.2), x86_64, PostgreSQL 16 running in Docker (localhost)

**Dataset:** 10,000,000 rows × ~600 B = ~6 GB uncompressed

**Configuration:**
- `stream_snapshot`: true
- `schema`: public
- `tables`: [cart]
- `slot_name`: bench_slot
- `batching.count`: 1000

**Throughput:**

```
INFO total stats: 211110.966072 msg/sec, 127 MB/sec    @service=redpanda-connect bytes/sec=1.2692347587018745e+08 label="" msg/sec=211110.96607224777 path=root.output.processors.0
```

Steady state: **~211,000 msg/sec, 127 MB/sec**, ~600 B per message.

**Observations:** With small rows the connector is no longer I/O bound on message size — throughput increases dramatically to 211K msg/sec. At this scale CPU becomes the limiting factor, making this dataset the right one for core-scaling and batch-size benchmarks.

---

## Snapshot — Small Rows (cart table), CPU Scaling

Same cart dataset, varying `GOMAXPROCS` to measure per-core throughput scaling.

### Run 1

**Environment:** Intel Core i7-10850H @ 2.70GHz, 32 GB RAM, WSL2 (Linux 6.6.87.2), x86_64, PostgreSQL 16 running in Docker (localhost)

**Dataset:** 10,000,000 rows × ~600 B

| GOMAXPROCS  | msg/sec | MB/sec |
|-------------|---------|--------|
| 1           | 134,287 |     81 |
| 2           | 212,852 |    128 |
| 4           | 276,259 |    166 |
| 8           | 300,760 |    181 |
| (unbounded) | 211,111 |    127 |

**Observations:** The connector scales with core count but with diminishing returns — 1→2 cores: 1.58x, 2→4 cores: 1.30x, 4→8 cores: 1.09x. Throughput is starting to plateau at 8 cores (300K msg/sec), suggesting the bottleneck is shifting from CPU to PostgreSQL read throughput or network overhead from the Docker socket.
