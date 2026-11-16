package render

import (
	"bytes"
	"fmt"
	"github.com/andrew-ships-stuff/impactr/internal/graph"
	"regexp"
	"strconv"
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

func TestMarkdownCarriesStats(t *testing.T) {
	r := Report{Impact: graph.Impact{Affected: []graph.Affected{{NodeID: "n05"}}}}
	r.After.Provenance.HeadSHA = "abc1234"
	r.Diff.Summary = graph.ChangeSummary{NodesAdded: 2, NodesModified: 1, EdgesAdded: 3}
	md := Markdown(r)
	if !strings.Contains(md, "<!-- impactr-comment:v1 -->") || !strings.Contains(md, "impactr-stats:v1 nodes=3 edges=3 affected=1 head=abc1234") {
		t.Fatalf("missing marker or stats line: %s", md)
	}
}

func chain() graph.Diff {
	var d graph.Diff
	for i := 0; i < 12; i++ {
		d.Nodes = append(d.Nodes, graph.Node{ID: fmt.Sprintf("n%02d", i), Name: fmt.Sprintf("Node %d", i), Kind: "function", Change: graph.Modified})
	}
	for i := 0; i < 11; i++ {
		d.Edges = append(d.Edges, graph.Edge{From: fmt.Sprintf("n%02d", i), To: fmt.Sprintf("n%02d", i+1), Kind: "calls"})
	}
	d.Edges = append(d.Edges, graph.Edge{From: "n03", To: "n00", Kind: "imports"}, graph.Edge{From: "n09", To: "n01", Kind: "imports"})
	return d
}

func TestSVGNodesInsideCanvas(t *testing.T) {
	var buf bytes.Buffer
	if e := SVG(&buf, chain(), 100, false); e != nil {
		t.Fatal(e)
	}
	s := buf.String()
	dims := regexp.MustCompile(`width="(\d+)" height="(\d+)" viewBox`).FindStringSubmatch(s)
	if dims == nil {
		t.Fatal("no viewBox")
	}
	width, _ := strconv.Atoi(dims[1])
	height, _ := strconv.Atoi(dims[2])
	nodes := regexp.MustCompile(`<rect x="(\d+)" y="(\d+)" width="`+strconv.Itoa(nodeW)+`" height="`+strconv.Itoa(nodeH)+`"`).FindAllStringSubmatch(s, -1)
	if len(nodes) != 12 {
		t.Fatalf("expected 12 node rects, got %d", len(nodes))
	}
	for _, m := range nodes {
		x, _ := strconv.Atoi(m[1])
		y, _ := strconv.Atoi(m[2])
		if x < 0 || y < 0 || x+nodeW > width || y+nodeH > height-legendH+20 {
			t.Fatalf("node rect %s,%s escapes canvas %dx%d", m[1], m[2], width, height)
		}
	}
}

func TestSVGEdgesStayInsideCanvas(t *testing.T) {
	var buf bytes.Buffer
	if e := SVG(&buf, chain(), 100, false); e != nil {
		t.Fatal(e)
	}
	s := buf.String()
	dims := regexp.MustCompile(`width="(\d+)" height="(\d+)" viewBox`).FindStringSubmatch(s)
	width, _ := strconv.Atoi(dims[1])
	height, _ := strconv.Atoi(dims[2])
	for _, m := range regexp.MustCompile(`M(\d+) (\d+) L(\d+) (\d+)`).FindAllStringSubmatch(s, -1) {
		for _, pair := range [][2]int{{1, 2}, {3, 4}} {
			x, _ := strconv.Atoi(m[pair[0]])
			y, _ := strconv.Atoi(m[pair[1]])
			if x < 0 || x > width || y < 0 || y > height {
				t.Fatalf("edge endpoint %d,%d outside canvas %dx%d", x, y, width, height)
			}
		}
	}
}
