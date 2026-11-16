package render

import (
	"fmt"
	"github.com/andrew-ships-stuff/impactr/internal/graph"
	"io"
	"testing"
)

func BenchmarkSVG(b *testing.B) {
	d := graph.Diff{}
	for i := 0; i < 250; i++ {
		d.Nodes = append(d.Nodes, graph.Node{ID: fmt.Sprintf("%024d", i), Name: fmt.Sprintf("Node%d", i), Kind: "function"})
		if i > 0 {
			d.Edges = append(d.Edges, graph.Edge{ID: fmt.Sprint(i), From: d.Nodes[i-1].ID, To: d.Nodes[i].ID, Kind: "calls"})
		}
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if e := SVG(io.Discard, d, 250, false); e != nil {
			b.Fatal(e)
		}
	}
}
