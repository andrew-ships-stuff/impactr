# Git hooks

```bash
impactr hook install --type pre-commit
impactr hook install --type pre-push
impactr hook install --type pre-push --blocking
impactr hook uninstall --type pre-push
```

Impactr preserves an existing executable hook as `<hook>.impactr-original`, installs a small chaining wrapper, and restores the original on uninstall. It refuses ambiguous backup states. Hooks run deterministic analysis without model downloads and are nonblocking by default. The existing hook's failure remains blocking.

The pre-commit hook analyzes the index. Pre-push uses the configured branch comparison. A deterministic quality gate is not enabled in v1; teams should inspect `report.json` in their own policy step rather than gate on LLM wording.
