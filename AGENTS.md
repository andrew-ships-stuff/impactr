# AGENTS.md

Instructions for AI coding agents working in this repository. Read this before changing anything. The human-facing introduction is in [README.md](README.md), contributor guidance is in [CONTRIBUTING.md](CONTRIBUTING.md), and trust boundaries are documented in [SECURITY.md](SECURITY.md).

## Project overview

Impactr is a local-first Go CLI that compares Git revisions, builds an evidence-backed architecture graph, computes a bounded blast radius, renders reviewable artifacts, and can optionally explain deterministic facts with CPU-only embedded inference. The CLI entry point is `cmd/impactr`; implementation packages live under `internal/`.

Core invariants agents must preserve:

- Deterministic analysis remains complete and useful without a model or network access.
- Keep confirmed graph facts separate from inference. AI claims must cite valid graph evidence IDs; invalid or unsupported claims are discarded, and provider failure falls back to a deterministic summary.
- Treat repository source and configuration as untrusted data. Impactr inspects code but does not run repository builds, tests, generators, scripts, or configuration commands.
- Git inspection must not mutate the analyzed working tree. Committed revisions use object archives; index analysis uses an isolated temporary directory.
- Stable node and edge IDs do not depend on line numbers. Preserve sorted, reproducible serialization and attach source evidence to every resolved relationship.
- `schema_version: 1.0.0` is the current artifact contract. Additive fields are allowed within 1.x; removals or breaking semantic changes require a new major version.
- Model downloads remain explicit for local users, license-disclosed, checksum-pinned, and atomically cached. Ordinary tests and CI must not download model weights.

## Concurrent agent rules (read first)

Multiple agents and humans may write to this working tree at the same time. Assume every pre-existing modification, staged path, and neighboring untracked file belongs to somebody else unless you created it or were explicitly assigned it.

1. Before editing, run `git status --short` and inspect the exact files you plan to touch. Re-check status throughout the task because another agent may change the tree while you work.
2. Never use `git add .`, `git add -A`, `git commit -a`, or wildcard staging. Stage only explicit files you changed, for example:
   ```bash
   git add internal/graph/view.go internal/graph/view_test.go
   git commit -m "Fix stable graph view ordering" -- internal/graph/view.go internal/graph/view_test.go
   ```
3. Before every commit, run `git status --short`, `git diff --cached`, and `git diff --cached --stat`. Commit only after confirming the index contains exactly your intended change. Use an explicit pathspec on `git commit` as a final guardrail.
4. Never reset, restore, stash, clean, reformat, or overwrite changes you did not create. Do not “tidy” unrelated files or the shared index.
5. If a required file already contains another contributor's work, inspect the diff and make a narrow compatible edit when safe. If ownership or intent is unclear, coordinate or leave the file untouched and report the collision.
6. Commit at logical milestones, not only at the end. Keep each commit small, coherent, independently reviewable, and passing the relevant tests. A finished test-backed behavior, a contained refactor, or a docs update paired with its behavior is a good milestone.
7. After committing, verify with `git status --short` and `git show --stat --oneline HEAD`. Do not push, amend, rebase, or otherwise rewrite shared history unless the user explicitly asks.
8. Never commit secrets, credentials, `.env` files, local configuration, downloaded model weights, or generated `.impactr/` analysis output. Review the staged diff for sensitive material before every commit.

## Setup, build, and verification

Impactr requires Go 1.26 or newer as declared in `go.mod`.

```bash
make build       # build ./bin/impactr
make test        # go test ./...
make race        # go test -race ./...
make vet         # go vet ./...
make check       # test + race + vet
make bench       # deterministic analysis/graph/render benchmarks
make example     # regenerate docs/images/example-impact.svg
```

Before committing, run formatting and the checks proportional to your change:

```bash
gofmt -l .       # must print nothing for Go files you touched
go test ./...
go vet ./...
```

Run `go test -race ./...` for concurrency, caching, filesystem, progress-reporting, or integration changes. Use package-focused tests during development, but run the full offline suite before handing off a code change. Keep online and real-model tests explicitly opt-in.

For website changes:

```bash
cd website
npm ci
npm run build
npm test
```

Consult `website/package.json` for the exact available scripts if these commands change.

## Testing instructions

- Add or update tests for every behavior change. Prefer fixture-backed expectations for analyzer changes and focused boundary tests for security-sensitive integrations.
- Tests must be deterministic, offline by default, and safe to run against an untrusted checkout. Do not invoke repository-owned scripts or fetch model weights.
- Preserve explicit limits: graph budgets, bounded workers, response-size caps, inference timeouts, and memory caps. Add tests at limit and failure boundaries when changing them.
- Analyzer facts need stable IDs, sorted output, and file/line evidence. Test unresolved syntax as unresolved rather than guessing a target.
- Inference tests must cover malformed output, unknown evidence IDs, provider failure, and deterministic fallback where relevant.
- Integration and artifact-transfer tests must exercise provenance and schema validation. Privileged publishing must never execute pull-request code.
- Benchmarks belong in the affected package when a hot path changes. Avoid weakening assertions or increasing limits merely to make a test pass.

## Code style

- Use standard `gofmt`. Keep Go straightforward and consistent with the surrounding package.
- Preserve the dependency direction: Git snapshots feed deterministic analyzers; graph data is the source of truth; renderers and inference consume versioned facts.
- Return contextual errors and handle cleanup failures where they affect correctness. Avoid panics for user-controlled input.
- Keep comments for invariants, trust boundaries, and genuinely non-obvious choices; do not narrate obvious code.
- Prefer narrow changes over repository-wide rewrites. Do not mechanically reformat files owned by another active agent.
- Pin external actions, binaries, and models deliberately. Never replace a checksum or explicit version with `latest` or another floating reference.

## Architecture map

- `cmd/impactr`: Cobra CLI wiring and terminal progress.
- `internal/app`: top-level orchestration of analysis, rendering, artifact writing, and integrations.
- `internal/git`: revision, merge-base, working-tree, and staged snapshot handling without checkout mutation.
- `internal/analysis`: language analyzers and bounded aggregation. Go analysis resolves packages, declarations, and statically named direct calls; JavaScript/TypeScript and Python are intentionally syntax-level.
- `internal/graph`: canonical nodes and edges, graph diffing, stable IDs, evidence, and bounded reverse-dependency impact traversal. This is the deterministic source of truth.
- `internal/render`: deterministic SVG/PNG/Mermaid/Markdown rendering with large-graph caps.
- `internal/inference`: embedded and external providers. It receives compact graph facts rather than repository files unless a user explicitly opts into source sharing.
- `internal/artifact`: report and image validation, schema compatibility, and transferred-artifact safety.
- `internal/integrations`: GitHub and GitLab publishing boundaries.
- `internal/hooks`: managed Git hook installation and execution.
- `internal/cache`: content/config/analyzer-keyed on-disk cache.
- `website/`: Astro documentation and marketing site.
- `testdata/session-service`: canonical before/after fixture used for examples and analyzer behavior.

## Security boundaries

- Escape repository-controlled text in every renderer and use structured encoders for JSON and HTTP bodies.
- Keep credentials in environment-backed headers; never serialize or log tokens.
- External inference is opt-in and should receive compact graph facts. Do not silently broaden what leaves the machine.
- Validate downloaded models against the disclosed SHA-256 before use, and retain atomic installation behavior.
- Keep untrusted analysis separate from privileged publication. The default-branch publisher must validate schema, images, PR head, and merge-base provenance before posting.
- Reject unknown major artifact versions. Validate transferred paths and content before reading or publishing them.

## Documentation and website synchronization

User-visible behavior changes require documentation in the same logical change:

- CLI commands, flags, defaults, configuration, or exit behavior: update `README.md`, `docs/configuration.md`, and applicable pages under `website/src/pages/docs/`.
- Inference providers, model behavior, downloads, licenses, or privacy: update `docs/inference.md`, `SECURITY.md` when a trust boundary changes, and matching website docs.
- GitHub/GitLab automation or artifact publishing: update the relevant file in `docs/` and its website counterpart.
- Graph semantics, artifact schemas, or renderer output: update `docs/architecture.md`, examples, and consumer-facing format documentation.

Never advertise behavior that is not implemented at the same revision. If the relevant docs file contains another agent's in-progress edits, do not overwrite them; report the exact follow-up needed.

## Commit conventions

- Make one logical change per commit and commit whenever a meaningful milestone is complete and verified.
- Use an imperative, descriptive subject no longer than 72 characters, such as `Fix stable graph view ordering` or `Add artifact provenance validation`.
- Pair implementation and its tests in the same commit. Include directly required documentation when it describes that exact behavior.
- Generated outputs such as `bin/`, `.impactr/`, local model caches, coverage files, and temporary benchmark data are not source inputs and must not be committed.
- At handoff, report the commits you created, the verification you ran, and any files or failures you intentionally left alone because they belong to concurrent work.
