package inference

import (
	"github.com/andrew-ships-stuff/impactr/internal/graph"
	"testing"
)

func TestValidationDropsUnsupportedClaims(t *testing.T) {
	r := Request{Diff: graph.Diff{Nodes: []graph.Node{{ID: "known", Change: graph.Modified}}}}
	e, err := Validate(Explanation{Summary: "x", Claims: []Claim{{Text: "bad", EvidenceIDs: []string{"made-up"}}, {Text: "ok", EvidenceIDs: []string{"known"}}}}, r)
	if err != nil || len(e.Claims) != 1 || e.Claims[0].Text != "ok" {
		t.Fatalf("%+v %v", e, err)
	}
}
