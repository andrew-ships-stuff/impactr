package render

import (
	"bytes"
	"github.com/andrew-ships-stuff/impactr/internal/graph"
	"strings"
	"testing"
)

func TestSVGDeterministic(t *testing.T) {
	d := graph.Diff{Nodes: []graph.Node{{ID: "a", Name: "A", Kind: "package", Change: graph.Added}}}
	var a, b bytes.Buffer
	_ = SVG(&a, d, 10, false)
	_ = SVG(&b, d, 10, false)
	if a.String() != b.String() || !strings.Contains(a.String(), "Legend") {
		t.Fatal("non-deterministic or missing legend")
	}
}
