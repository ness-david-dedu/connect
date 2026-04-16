# Kafka Connect JDBC Sink — PostgreSQL Benchmark

Benchmarks Kafka Connect (Confluent JDBC Sink) reading from Kafka and writing to PostgreSQL. Use the results alongside the parent `bench/` Redpanda Connect benchmark to compare throughput.

## Architecture

```
Redpanda Connect producer → bench-events (16 partitions) → Kafka Connect JDBC Sink → PostgreSQL bench_events
```

- **Kafka** — KRaft broker, no ZooKeeper
- **PostgreSQL 16** — sink target (`benchdb.bench_events`)
- **Kafka Connect** — JDBC Sink connector (`confluentinc/kafka-connect-jdbc:10.7.14`)
- **Message format** — JSON with embedded Kafka Connect schema envelope (no Schema Registry needed)

## Prerequisites

- Docker
- `task` (Taskfile runner)
- `psql` client (for row-count verification)
- `go` (to run the producer via Redpanda Connect)

## Quickstart

```bash
# 1. Start all infrastructure
task up

# 2. Run a full benchmark (loads 1M events, registers connector, measures throughput)
task bench:run COUNT=1000000

# 3. Tear down
task down
```

## Step-by-step

```bash
# Start infrastructure and wait for Kafka Connect to be ready
task up

# Load events into Kafka (produces COUNT messages with embedded JSON schema)
task data:load COUNT=1000000

# Register the JDBC sink connector
task connector:create

# Monitor throughput until the topic is fully drained
task bench:measure TOTAL=1000000 INTERVAL=5

# Verify PostgreSQL row count matches
task bench:pg-count-verify EXPECTED=1000000
```

## Connector Configuration

Key settings in `connector.json`:

| Setting | Value | Notes |
|---|---|---|
| `tasks.max` | 16 | Match Kafka topic partition count |
| `insert.mode` | `insert` | Append-only inserts |
| `batch.size` | 3000 | Rows per JDBC batch |
| `table.name.format` | `bench_events` | Explicit table name (avoids hyphen quoting) |
| `value.converter.schemas.enable` | `true` | Reads schema from message envelope |

## Tasks Reference

| Task | Description |
|---|---|
| `task up` | Start all containers, wait for Connect readiness |
| `task down` | Stop and remove all containers and volumes |
| `task data:load COUNT=N` | Produce N events to bench-events |
| `task connector:create` | Register the JDBC sink connector |
| `task connector:status` | Show connector and task status |
| `task connector:delete` | Delete the connector (keeps infrastructure) |
| `task bench:run COUNT=N` | Full benchmark: load + measure |
| `task bench:measure TOTAL=N INTERVAL=S` | Poll lag and print msg/s until drained |
| `task bench:lag` | Show current consumer group lag |
| `task bench:pg-count` | Count rows in bench_events table |
| `task bench:pg-count-verify EXPECTED=N` | Assert row count equals expected |
| `task bench-topic:recreate` | Delete and recreate bench-events topic |
| `task logs:connect` | Follow Kafka Connect logs |
| `task logs:postgres` | Follow PostgreSQL logs |

## Expected Output

```
TIME        LAG             MSG/S
09:01:15    1000000         (timing started)
09:01:20    850000          30000 msg/s
09:01:25    690000          32000 msg/s
09:01:30    520000          34000 msg/s
...
09:01:55    0               31000 msg/s
---
Total messages : 1000000
Elapsed        : 40s
Avg throughput : 25000 msg/s
```

## Tuning

- **`tasks.max`** in `connector.json` — increase to match partition count for maximum parallelism
- **`batch.size`** in `connector.json` — larger batches reduce round-trips; PostgreSQL typically handles 3000–10000 well
- **`COUNT`** in producer — use 5M+ messages for stable throughput measurements
- **`INTERVAL`** in `bench:measure` — keep ≥ 5s; the JDBC sink flushes in batches and lag appears frozen between flushes
