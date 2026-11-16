# GitHub Action

The Action builds Impactr from the trusted action checkout, analyzes base/head SHAs with no inference by default, adds a job summary, attaches a PNG to one bot comment for same-repository PRs, and always uploads the complete artifact bundle.

Use `pull_request`, never `pull_request_target` with a checkout of the PR head. Fork PR tokens are read-only and receive no secrets, so the composite Action skips its publishing step. For a privileged follow-up, use a separate `workflow_run` workflow from the default branch that:

1. downloads only the named Impactr artifact for the completed run;
2. caps the archive/download size before extraction and rejects traversal/symlinks;
3. runs `impactr validate-artifact --dir <dir>` (25 MiB bundle, 10 MiB JSON, schema, PNG dimension, and marker checks);
4. verifies the workflow repository and PR/head identity through GitHub's API, not artifact fields;
5. runs `impactr github comment --existing --pr N --repo owner/repo --output <dir>` without checking out or executing fork code.

Transferred files are untrusted data. Never source them, execute them, interpolate them into a shell command, or honor a configuration file from the artifact. `pull-requests: write` is needed only in the publishing trust context. Artifact-only analysis needs `contents: read`.

`gh pr comment --attach` rewrites the local `impact.png` reference to a durable GitHub attachment. The CLI uses `--edit-last --create-if-none`; dedicate the publishing identity to Impactr so its last comment is the identifiable `<!-- impactr-comment:v1 -->` comment.
