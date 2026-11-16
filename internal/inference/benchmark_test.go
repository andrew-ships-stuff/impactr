package inference

import (
	"context"
	"github.com/andrew-ships-stuff/impactr/internal/graph"
	"os"
	"runtime"
	"testing"
)

func BenchmarkEmbeddedInference(b *testing.B) {
	path := os.Getenv("IMPACTR_BENCH_MODEL")
	if path == "" {
		b.Skip("set IMPACTR_BENCH_MODEL to a verified GGUF for the online benchmark")
	}
	p := Embedded{ModelPath: path, ModelName: "benchmark", Threads: max(1, runtime.GOMAXPROCS(0)/2), ContextLength: 1024, MaxMemoryMB: 2048}
	req := Request{Diff: graph.Diff{Nodes: []graph.Node{{ID: "a", Name: "A", Change: graph.Modified}}}, Impact: graph.Impact{Changed: []string{"a"}}, MaxTokens: 64}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, e := p.Explain(context.Background(), req); e != nil {
			b.Fatal(e)
		}
	}
}
