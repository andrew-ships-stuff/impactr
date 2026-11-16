# Architecture

Impactr has one dependency direction: adapters feed the deterministic engine; renderers and inference consume its versioned facts.

```text
Git snapshots -> language analyzers -> canonical graph -> graph diff -> blast radius
                                                |             |
                                                +-> renderers  +-> inference
                                                                |
                                            hooks / GitHub / GitLab adapters
```

`internal/git` resolves exact or merge-base comparisons and reads commits with `git archive`; it never checks out a revision. The index uses `git checkout-index` into an isolated temporary directory. `internal/analysis` selects language analyzers and aggregates sorted results under a bounded worker limit. Content/config/analyzer hashes key the on-disk cache.

`internal/graph` is the source of truth. Node IDs hash language, canonical package, qualified symbol, and kind—not line numbers. Edge IDs hash endpoints and relationship kind. Function body hashes classify implementation changes independently from relationships. Every resolved fact has source evidence; unresolved syntax stays explicitly unresolved.

The renderer uses an ID-derived deterministic grid so unchanged identities tend to retain positions. SVG is first-class; PNG uses a native Go rasterizer. Large graphs are capped, while the machine-readable graph remains complete.

Inference receives compact changed facts and impact IDs, not repository files. Claims without valid graph evidence IDs are discarded. Provider failure yields a deterministic summary.

## Schema compatibility

All JSON envelopes carry `schema_version: 1.0.0`. Additive fields are allowed within 1.x. A breaking semantic or field removal requires 2.0. Artifact consumers must reject unknown major versions.
