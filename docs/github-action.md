# GitHub Action

The Action builds Impactr from the trusted action checkout, analyzes explicit base/head SHAs, uses cached embedded inference by default, adds a job summary, and always uploads the complete artifact bundle. Set `provider: none` for deterministic-only CI.

Use `pull_request`, never `pull_request_target` with a checkout of the PR head. Fork PR tokens are read-only and receive no secrets, so the composite Action skips its publishing step. This repository implements the privileged follow-up in `.github/workflows/publish-impact.yml`. It runs only after the read-only `CI` workflow succeeds and:

1. downloads only the named Impactr artifact for the completed run;
2. treats every extracted file as data and rejects symlinks, non-regular files, oversized bundles, and unexpected image names;
3. obtains the current PR head and merge-base through GitHub's API and runs `impactr validate-artifact --dir <dir> --base <merge-base> --head <head>` (25 MiB bundle, 10 MiB JSON, schema, every PNG payload/dimension, marker, and provenance checks);
4. verifies the workflow-run head matches the current PR head, preventing a stale run from overwriting newer analysis;
5. runs `impactr github comment --existing --pr N --repo owner/repo --output <dir>` without checking out or executing fork code.

Transferred files are untrusted data. Never source them, execute them, interpolate them into a shell command, or honor a configuration file from the artifact. `pull-requests: write` is needed only in the publishing trust context, plus `contents: write` there for the diagram (the GitHub uploads endpoint rejects `GITHUB_TOKEN` with 404, so Impactr falls back to committing `impact.png` to a dedicated `impactr-assets` branch and referencing it through a raw URL; with a classic repo-scoped token the uploads endpoint is used instead). Artifact-only analysis needs `contents: read`.

This repository dogfoods the Action: `dogfood` in `.github/workflows/ci.yml` runs `uses: ./` on the PR's own code with `contents: read` and `provider: none`. The `workflow_run` publisher checks out and builds only the default branch, never the PR branch. That separation also supports forks without granting their code a write-capable token.

The composite Action defaults to `provider: embedded`. Selecting it is the non-interactive CI consent boundary: the Action displays the Apache-2.0 model metadata, downloads the checksum-pinned model into the Actions cache, and then reuses it offline. Apache-2.0 does not require a click-through acceptance. Set `provider: none` for deterministic-only CI with no model download.

`gh pr comment --attach` rewrites the local `impact.png` reference to a durable GitHub attachment. Each comment carries a `<!-- impactr-comment:v1 -->` marker plus a machine-readable `<!-- impactr-stats:v1 nodes=N edges=M affected=A head=SHA -->` line. On republish, Impactr finds its latest marked comment through the API and edits it in place, keeping one canonical comment per PR. If any metric more than doubled since that comment (in either direction, with a slack of 5), the change is considered substantial and a new comment is posted instead, marked as superseding the previous analysis, so a sudden blast-radius explosion is never silently overwritten. `--edit-last` identifies the comment to edit; dedicate the publishing identity to Impactr.
