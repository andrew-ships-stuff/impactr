# GitHub Action

The Action builds Impactr from the trusted action checkout, analyzes base/head SHAs with no inference by default, adds a job summary, attaches a PNG to one bot comment for same-repository PRs, and always uploads the complete artifact bundle.

Use `pull_request`, never `pull_request_target` with a checkout of the PR head. Fork PR tokens are read-only and receive no secrets, so the composite Action skips its publishing step. For a privileged follow-up, use a separate `workflow_run` workflow from the default branch that:

1. downloads only the named Impactr artifact for the completed run;
2. caps the archive/download size before extraction and rejects traversal/symlinks;
3. runs `impactr validate-artifact --dir <dir>` (25 MiB bundle, 10 MiB JSON, schema, PNG dimension, and marker checks);
4. verifies the workflow repository and PR/head identity through GitHub's API, not artifact fields;
5. runs `impactr github comment --existing --pr N --repo owner/repo --output <dir>` without checking out or executing fork code.

Transferred files are untrusted data. Never source them, execute them, interpolate them into a shell command, or honor a configuration file from the artifact. `pull-requests: write` is needed only in the publishing trust context. Artifact-only analysis needs `contents: read`.

This repository dogfoods the Action: `dogfood` in `.github/workflows/ci.yml` runs `uses: ./` on the PR's own code with `contents: read` and `provider: none`, and the separate `publish-impact-comment` job (same-repository PRs only) rebuilds a trusted binary, validates the artifact bundle, and runs `impactr github comment --existing`.

`gh pr comment --attach` rewrites the local `impact.png` reference to a durable GitHub attachment. Each comment carries a `<!-- impactr-comment:v1 -->` marker plus a machine-readable `<!-- impactr-stats:v1 nodes=N edges=M affected=A head=SHA -->` line. On republish, Impactr finds its latest marked comment through the API and edits it in place, keeping one canonical comment per PR. If any metric more than doubled since that comment (in either direction, with a slack of 5), the change is considered substantial and a new comment is posted instead, marked as superseding the previous analysis, so a sudden blast-radius explosion is never silently overwritten. `--edit-last` identifies the comment to edit; dedicate the publishing identity to Impactr.
