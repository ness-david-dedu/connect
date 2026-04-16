# Kafka → Iceberg Benchmark (Kafka Connect vs Redpanda Connect)

End-to-end benchmark comparing **Kafka Connect (Tabular Iceberg Sink)** against **Redpanda Connect** writing from Kafka to Iceberg. Both connectors apply the same field transformation and use the same infrastructure.

See [`docs/benchmark-results/iceberg.md`](../../../../../docs/benchmark-results/iceberg.md) for results.

## Prerequisites

- Docker with Compose
- Go toolchain
- [`task`](https://taskfile.dev) CLI
- `jq`

## Infrastructure

```bash
task up     # start Kafka, MinIO, Iceberg REST, Kafka Connect
task down   # stop and remove all containers and volumes
```

| Service | URL |
|---------|-----|
| Kafka | localhost:9092 |
| Kafka Connect | http://localhost:8083 |
| Iceberg REST catalog | http://localhost:18181 |
| MinIO console | http://localhost:19001 (admin/password) |

## Kafka Connect Benchmark

Full end-to-end run: produce → transform → sink into Iceberg.

```bash
task bench:run COUNT=10000000
```

This will:
1. Delete the existing connector and reset offsets
2. Produce `COUNT` events to `bench-events`
3. Run RPCN to transform `bench-events` → `bench-events-transformed`
4. Register the Kafka Connect Iceberg sink and measure throughput

### Individual steps

```bash
task data:load COUNT=1000000        # produce events to bench-events
task bench:transform                # transform bench-events → bench-events-transformed
task bench:sink                     # register connector and measure throughput
```

### Monitoring

```bash
task bench:lag                      # show current consumer lag
task bench:offsets                  # show end offsets for both topics
task connector:status               # show connector and task status
task logs:connect                   # follow Kafka Connect worker logs
```

### Data management

```bash
task data:stats                     # show Iceberg table snapshot stats
task data:reset                     # drop and recreate the Iceberg table
task bench-topic:recreate           # delete and recreate bench-events topic
task control-topic:reset            # reset the Iceberg control topic
```

## Redpanda Connect Benchmark

Run from the parent folder ([`bench/`](../)):

```bash
cd ..
task bench:run COUNT=10000000
```

This runs a single pipeline: Kafka → transform → Iceberg, with no intermediate topic.
