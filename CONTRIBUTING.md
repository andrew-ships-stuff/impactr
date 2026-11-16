# Contributing

Use Go 1.24 or newer. Keep deterministic facts separate from inference, preserve stable IDs and sorted serialization, and attach evidence to every new relationship.

```bash
go test ./...
go test -race ./...
go vet ./...
go test -bench . -benchmem ./internal/analysis ./internal/graph ./internal/render
```

Online/model tests must be opt-in and must not make ordinary CI download weights. Add fixture-backed expectations for analyzer changes and security-boundary tests for integrations.
