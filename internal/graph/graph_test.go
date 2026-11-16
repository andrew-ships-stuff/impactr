package graph

import "testing"

func TestStableIDAndBodyOnlyDiff(t *testing.T) {
	a := StableNodeID("go", "p", "p.F", "function")
	b := StableNodeID("go", "p", "p.F", "function")
	if a != b {
		t.Fatal("unstable")
	}
	before := Graph{Nodes: []Node{{ID: a, BodyHash: "a"}}}
	after := Graph{Nodes: []Node{{ID: a, BodyHash: "b"}}}
	d := Compare(before, after)
	if d.Summary.NodesModified != 1 || d.Summary.EdgesAdded != 0 {
		t.Fatalf("bad diff: %+v", d.Summary)
	}
}
func TestBlastRadiusCycleBounded(t *testing.T) {
	d := Diff{Nodes: []Node{{ID: "a", Change: Modified}, {ID: "b"}, {ID: "c"}}, Edges: []Edge{{ID: "1", From: "b", To: "a", Resolution: Resolved}, {ID: "2", From: "c", To: "b", Resolution: Resolved}, {ID: "3", From: "a", To: "c", Resolution: Resolved}}}
	got := BlastRadius(d, 4, 10)
	if len(got.Affected) != 2 {
		t.Fatalf("got %d", len(got.Affected))
	}
}
