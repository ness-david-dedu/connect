# AWS S3 Benchmark Results

Benchmark suite: `internal/impl/aws/s3/bench/`

## 2026-04 — Initial Benchmark

**Environment:**
- LocalStack running in Docker (`localstack/localstack:3`)
- Local development machine
- `GOMAXPROCS=4`

**Dataset:** 50,000 objects (~1 KB each)
- Single bucket `bench-objects`
- Objects: JSON payloads with padded `data` field

**Configuration:**
- Default `aws_s3` input (no SQS, bucket walk mode)
- `force_path_style_urls: true`, endpoint `http://localhost:4566`

**Results:**

| GOMAXPROCS | Throughput   | Messages/sec |
|------------|--------------|--------------|
| 1          | ~402 KB/sec  | ~393         |
| 2          | ~428 KB/sec  | ~418         |
| 4          | ~457 KB/sec  | ~446         |
| 8          | ~438 KB/sec  | ~428         |


**Observations:**
- The `aws_s3` input processes one object at a time (sequential `GetObject` calls) — throughput is bounded by per-object HTTP round-trip latency, not data size or CPU
- Increasing GOMAXPROCS from 4 to 8 made no difference (within noise), confirming the bottleneck is serial I/O not CPU
- LocalStack runs in-process with negligible network overhead; real AWS S3 latency (~few ms per request) would reduce throughput further
- For high-throughput S3 ingestion the SQS-triggered mode or multiple parallel Connect instances would be needed
- Increasing object size (e.g. `--size 65536`) would improve MB/sec while keeping msg/sec roughly the same, since the bottleneck is request count not data volume
