# GitLab CI

Include `.gitlab/impactr.gitlab-ci.yml` or copy its job. Merge-request pipelines use the same CLI, graph schema, and renderer. `CI_MERGE_REQUEST_DIFF_BASE_SHA` and `CI_COMMIT_SHA` are exact endpoints.

Set a masked, protected `GITLAB_TOKEN` with the minimum API access needed for project uploads and merge-request notes. Without it, analysis and artifacts still succeed. The integration uploads the PNG, finds the existing note marker, and updates that note instead of duplicating it. Be aware that project-upload media visibility depends on GitLab instance settings.

Do not expose protected variables to untrusted fork pipelines. Use an artifact-only job for those pipelines and a separately trusted publisher after validating the bundle and pipeline identity.
