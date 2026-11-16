# Third-party notices

- `github.com/goccy/go-llama` — MIT; includes a derivative of `llama.cpp` (MIT).
- `github.com/goccy/llamawasm2go` — MIT.
- `github.com/spf13/cobra` and its transitive CLI dependencies — Apache-2.0/MIT as recorded in their distributions.
- `gopkg.in/yaml.v3` — MIT/Apache-2.0.
- `golang.org/x/image` — BSD-3-Clause.
- Default optional model: Qwen2.5-0.5B-Instruct GGUF — Apache-2.0; weights are downloaded separately and are not redistributed in Impactr binaries.

Run `go version -m <impactr-binary>` to inspect the exact compiled Go module versions. Release archives do not contain model weights.
