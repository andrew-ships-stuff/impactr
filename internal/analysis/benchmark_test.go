package analysis

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkGoAnalysis(b *testing.B) {
	root := b.TempDir()
	_ = os.WriteFile(filepath.Join(root, "go.mod"), []byte("module bench.test/app\n"), 0644)
	for i := 0; i < 100; i++ {
		body := fmt.Sprintf("package p\nfunc F%d() int { return %d }\n", i, i)
		_ = os.WriteFile(filepath.Join(root, fmt.Sprintf("f%03d.go", i)), []byte(body), 0644)
	}
	eng := New()
	opt := Options{Languages: []string{"go"}, Workers: 4}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, e := eng.Analyze(context.Background(), root, "bench", opt); e != nil {
			b.Fatal(e)
		}
	}
}
