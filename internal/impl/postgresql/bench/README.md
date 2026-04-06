# Benchmarking PostgreSQL CDC Component

Benchmark demonstrating throughput of Redpanda's PostgreSQL CDC Connector.

## Prerequisites

Install `psql`:

```bash
# macOS
brew install postgresql

# Ubuntu/Debian
sudo apt install postgresql-client
```

## Setup

1. Start PostgreSQL container:

```bash
task postgres:up
```

This starts a PostgreSQL 16 container with `wal_level=logical` enabled (required for CDC).

2. Create tables:

```bash
task psql:create
```

3. Insert test data:

```bash
task psql:data:users     # 150K rows, ~625KB per row (large payload)
task psql:data:products  # 150K rows, ~625KB per row (large payload)
task psql:data:cart      # 10M rows, small payloads
```

## Benchmark Tasks

### Snapshot benchmarks

Run a full snapshot of pre-inserted data (`stream_snapshot: true`):

```bash
task bench:run                # run with current table data
task bench:snapshot:users     # truncate + insert 150K large rows + run
task bench:snapshot:cart      # truncate + insert 10M small rows + run
task bench:snapshot:all       # truncate + insert all datasets + run
```

### CPU scaling

Vary `GOMAXPROCS` to measure per-core throughput. Most useful with the cart dataset (small rows) where CPU rather than I/O is the bottleneck:

```bash
task bench:cores:1
task bench:cores:2
task bench:cores:4
task bench:cores:8
```

Run the cart scenario for each core count (reuses existing data, just drops the slot between runs):

```bash
task bench:cart:cores:1
task bench:cart:cores:2
task bench:cart:cores:4
task bench:cart:cores:8
```

### Batch size

Vary `batching.count` to find the throughput/latency sweet spot:

```bash
task bench:batch:100
task bench:batch:500
task bench:batch:1000   # default
task bench:batch:5000
```

### Parallel snapshot tables

Vary `max_parallel_snapshot_tables` (only meaningful when all 3 tables are populated):

```bash
task bench:parallel-tables:1
task bench:parallel-tables:2
task bench:parallel-tables:3
```

### CDC mode

Start the connector first (it creates the replication slot), then insert data in a second terminal while it is running:

```bash
# terminal 1
task bench:run:cdc

# terminal 2 — once the connector is up
task psql:data:users
```

## Between Runs

Drop the replication slot before re-running any benchmark so the connector replays from the start:

```bash
task psql:drop-slot
```

> Unused replication slots cause PostgreSQL to retain WAL segments indefinitely, which can fill up disk. Always drop the slot when done.

## Expected Output

```
INFO rolling stats: 1000 msg/sec, 625 MB/sec    @service=redpanda-connect bytes/sec=6.2527229e+08 label="" msg/sec=1000 path=root.output.processors.0
INFO rolling stats: 2000 msg/sec, 1.3 GB/sec    @service=redpanda-connect bytes/sec=1.25054458e+09 label="" msg/sec=2000 path=root.output.processors.0
INFO rolling stats: 1000 msg/sec, 625 MB/sec    @service=redpanda-connect bytes/sec=6.2527229e+08 label="" msg/sec=1000 path=root.output.processors.0
```

Throughput with large rows (~625 KB each) is I/O bound — CPU core count has little effect. Use the cart dataset or add a processor to the pipeline to stress CPU.
