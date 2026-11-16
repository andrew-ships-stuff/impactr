# Inference

The `InferenceProvider` boundary supports:

- `embedded`: `github.com/goccy/go-llama`, a pure-Go, static-binary llama.cpp derivative. This avoids CGo and an external server, caps linear memory, sets inference threads, and supports cancellation.
- `ollama`: local `/api/chat`, defaulting to `127.0.0.1:11434`.
- `llama-server`: OpenAI-compatible local llama.cpp server, defaulting to `127.0.0.1:8080`.
- `openai-compatible`: a configured endpoint; credentials use `IMPACTR_API_KEY`.
- `none`: deterministic facts only.

Impactr sends compact JSON containing changed nodes, changed edges, and bounded affected-node IDs. It does not send full source. External providers therefore receive names, paths, signatures, evidence summaries, and graph relationships; using one is an explicit configuration choice.

The default model registry pins Qwen2.5-0.5B-Instruct Q4_0 to SHA-256 `7671c0c304e6ce5a7fc577bcb12aba01e2c155cc2efd29b2213c95b18edaf6ed`. Downloads use a partial file and atomic rename after verification. The manager supports offline reuse.

Generated output is JSON and each claim must cite existing node or edge IDs. Unsupported claims are removed. This is validation against evidence identity, not a proof that natural-language wording is perfect; reviewers should follow the linked facts.
