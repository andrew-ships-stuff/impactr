# Security

Please report vulnerabilities privately through GitHub Security Advisories.

Impactr parses repository content as untrusted data. It does not run builds, tests, generators, repository scripts, or configuration commands. Git inspection uses object archives or isolated index snapshots and never changes the working tree. SVG text is escaped; HTTP bodies use JSON encoders; credentials stay in environment-backed headers.

Primary trust boundaries are repository source/configuration, downloaded GGUF weights, external inference endpoints, and CI artifact transfer. Models are license-disclosed and checksum-pinned. External providers are opt-in and receive compact graph facts. The repository's privileged `workflow_run` publisher checks out only the default branch, validates every transferred image and the report schema, and binds report provenance to the current PR head and merge-base before publishing. It never executes PR code.

Known residual risks include parser resource exhaustion on adversarially large source and model/runtime defects. Analysis has graph budgets, bounded workers, HTTP response limits, inference timeouts, and memory caps; CI should additionally apply job time and memory limits.
