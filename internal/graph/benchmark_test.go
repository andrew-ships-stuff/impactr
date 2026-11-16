package graph

import (
	"fmt"
	"testing"
)

func BenchmarkBlastRadius(b *testing.B) {
	d := Diff{}
	for i := 0; i < 5000; i++ {
		change := Unchanged
		if i == 0 {
			change = Modified
		}
		d.Nodes = append(d.Nodes, Node{ID: fmt.Sprint(i), Change: change})
		if i > 0 {
			d.Edges = append(d.Edges, Edge{ID: fmt.Sprint(i), From: fmt.Sprint(i), To: fmt.Sprint(i - 1), Resolution: Resolved})
		}
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = BlastRadius(d, 4, 250)
	}
}
