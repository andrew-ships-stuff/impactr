# Design research

Impactr was designed after reviewing [gitVisualise](https://github.com/Kaushik2210/gitVisualise), [PR Lens](https://github.com/jasonkneen/pr-lens-with-canvas), [go-llama](https://github.com/goccy/go-llama), [llama.cpp](https://github.com/ggml-org/llama.cpp), and [Ollama](https://github.com/ollama/ollama). No source or assets were copied.

Useful patterns retained at the design level are gitVisualise's local/browser-friendly scanning and exportable tours, and PR Lens's narrow-review diagrams, stable comment lifecycle, explicit change colors, and renderer/schema separation. Impactr differs by making static evidence the graph authority and inference an optional consumer rather than the graph generator.

For embedded inference, direct llama.cpp bindings offer excellent native performance but add CGo, platform toolchains, and release-matrix complexity. `go-llama` packages a llama.cpp derivative as pure Go with static builds, cancellation, memory caps, and arm64/amd64 SIMD; it was selected for the default binary. llama.cpp's OpenAI-compatible server and Ollama remain optional HTTP adapters for users who prioritize their existing runtime. All three upstream projects use permissive licenses; model weights retain their own disclosed license.

GitHub CLI's attachment support is used because ordinary Actions artifacts do not render as Markdown images. Fork analysis and publishing are split by trust: read-only analysis may process untrusted source, while any write-token publisher accepts only bounded validated artifacts and never executes PR code.
