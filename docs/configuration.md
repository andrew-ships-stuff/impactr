# CLI and configuration

Run `impactr help` or `impactr <command> --help` for the generated command reference. Main commands are `analyze`, `render`, `model install`, `hook install|uninstall`, `github comment`, `gitlab comment`, and `validate-artifact`.

Configuration precedence is CLI flags, `IMPACTR_*` environment variables, project `.impactr.yaml`, user config (`$XDG_CONFIG_HOME/impactr/config.yaml` or the OS equivalent), then built-in defaults. Environment keys currently supported are `IMPACTR_PROVIDER`, `IMPACTR_MODEL`, `IMPACTR_ENDPOINT`, `IMPACTR_API_KEY`, and `IMPACTR_MODEL_CACHE`. Credentials are read only from the environment and are never logged.

The checked-in [.impactr.yaml](../.impactr.yaml) is a complete example. Unknown YAML fields and unsupported schema versions are rejected. No configuration field executes commands.

Comparison modes:

- `merge-base` compares the merge base of base/head to head and is the PR default.
- `exact` compares the exact endpoints.
- `--staged` compares HEAD to the index.
- `--working-tree` compares HEAD to the current tree. Untracked inclusion is reserved for a later release and is currently not enabled.

Default-branch discovery checks `origin/HEAD`, then conventional local/remote main/master names. Missing refs and shallow-history merge-base failures are errors with fetch guidance; Impactr never silently selects an unrelated commit.
