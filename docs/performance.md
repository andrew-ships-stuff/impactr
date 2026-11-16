# Performance and benchmarks

Impactr bounds analyzer concurrency (default `min(GOMAXPROCS, 16)`, maximum 32), sorts all canonical output, caches graphs by source/config/analyzer content hashes, and runs before/after analyzers concurrently. Embedded inference gets a separate configurable thread budget so users can avoid CPU oversubscription.

Run reproducible deterministic benchmarks with:

```bash
go test -run '^$' -bench . -benchmem ./internal/analysis ./internal/graph ./internal/render
```

The source benchmark parses 100 Go files, the graph benchmark traverses a 5,000-node chain with depth/budget bounds, and the render benchmark produces a 250-node SVG. Results below were measured, not estimated, on the stated checkout. Peak RSS and CPU utilization are not emitted by Go's benchmark harness; use `/usr/bin/time -l` or Linux `time -v` around the command when those metrics are required.

Embedded inference is intentionally excluded from offline CI. To measure real model initialization and generation on a local verified GGUF:

```bash
IMPACTR_BENCH_MODEL=/path/to/model.gguf go test -run '^$' -bench Embedded -benchtime=1x ./internal/inference
```

Record model hash, quantization, Go version, OS/architecture, CPU, thread count, context, time to first token, tokens/second, and peak RSS when publishing results. Impactr does not ship fabricated cross-platform claims.

## Recorded deterministic baseline

Measured 2026-11-15 on Apple M5, macOS arm64, Go 1.26.2 (`-benchmem`; benchmark count chosen automatically):

| Benchmark | Time/op | Bytes/op | Allocs/op |
|---|---:|---:|---:|
| Go analysis, 100 files | 3.057 ms | 900,278 | 10,920 |
| Blast radius, 5,000 nodes | 0.380 ms | 1,424,204 | 5,077 |
| SVG render, 250 nodes | 0.268 ms | 164,967 | 4,408 |

Embedded benchmark on the same machine with the checksum-pinned Qwen2.5-0.5B-Instruct Q4_0, 1,024-token context, 5 threads, 64-token cap, GBNF-constrained JSON, and one complete engine/model/context initialization plus structured generation: **4.927 s/op**, 553,040 Go bytes/op, 7,374 Go allocations/op. GGUF/native linear-memory allocations are not fully represented by `-benchmem`. The benchmark produced evidence-valid JSON. This single sample is a functional baseline, not a latency distribution; time-to-first-token, peak RSS, and tokens/second remain unmeasured by the current provider API.

An end-to-end working-tree analysis after adding package briefs used one shared model load for both constrained generations (4 threads, 4,096-token context): model initialization **0.36 s**, explanation **24.84 s / 64 streamed pieces**, brief **10.93 s / 73 streamed pieces**, and **37.06 s** wall time overall (`user 133.30 s`, `sys 0.98 s`). The brief failed the grounded/concise output validator and cleanly fell back to deterministic prose. “Pieces” are streaming callbacks, not tokenizer tokens; the figures should not be presented as token throughput.
