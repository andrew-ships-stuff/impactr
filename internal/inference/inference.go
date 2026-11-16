package inference

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/andrew-ships-stuff/impactr/internal/graph"
	"sort"
	"strings"
)

type Request struct {
	Diff      graph.Diff   `json:"diff"`
	Impact    graph.Impact `json:"impact"`
	MaxTokens int          `json:"-"`
}
type Claim struct {
	Text           string           `json:"text"`
	EvidenceIDs    []string         `json:"evidence_ids"`
	Classification graph.Resolution `json:"classification"`
}
type Explanation struct {
	Summary  string  `json:"summary"`
	Claims   []Claim `json:"claims"`
	Provider string  `json:"provider"`
	Model    string  `json:"model,omitempty"`
	Fallback bool    `json:"fallback"`
}
type Provider interface {
	Name() string
	Explain(context.Context, Request) (Explanation, error)
}

func Facts(req Request) []byte {
	type nodeFact struct {
		ID, Kind, Name, Path, Signature string
		Change                          graph.Change
		Resolution                      graph.Resolution
	}
	type edgeFact struct {
		ID, From, To, Kind string
		Change             graph.Change
		Resolution         graph.Resolution
	}
	type compact struct {
		ChangedNodes []nodeFact       `json:"changed_nodes"`
		ChangedEdges []edgeFact       `json:"changed_edges"`
		Affected     []graph.Affected `json:"potentially_affected"`
	}
	c := compact{Affected: req.Impact.Affected}
	for _, n := range req.Diff.Nodes {
		if n.Change != graph.Unchanged {
			c.ChangedNodes = append(c.ChangedNodes, nodeFact{ID: n.ID, Kind: n.Kind, Name: n.QualifiedName, Path: n.Location.Path, Signature: n.Signature, Change: n.Change, Resolution: n.Resolution})
		}
	}
	for _, e := range req.Diff.Edges {
		if e.Change != graph.Unchanged {
			c.ChangedEdges = append(c.ChangedEdges, edgeFact{ID: e.ID, From: e.From, To: e.To, Kind: e.Kind, Change: e.Change, Resolution: e.Resolution})
		}
	}
	b, _ := json.Marshal(c)
	return b
}
func Prompt(req Request) string {
	return fmt.Sprintf(`Explain this architecture change using ONLY the JSON facts. The static counts are: %d nodes added, %d removed, %d modified, %d edges added, %d removed. Output one JSON object with keys "summary" and "claims". Summary must be a concrete sentence naming important changed symbols; never output instructions or placeholder phrases. Each claim has keys "text", "evidence_ids", and "classification" (resolved, potential, or unresolved). Cite only IDs present in facts. Never invent dependencies. Reachable impact is potential, not confirmed. Facts: %s`, req.Diff.Summary.NodesAdded, req.Diff.Summary.NodesRemoved, req.Diff.Summary.NodesModified, req.Diff.Summary.EdgesAdded, req.Diff.Summary.EdgesRemoved, string(Facts(req)))
}
func Validate(e Explanation, req Request) (Explanation, error) {
	valid := map[string]bool{}
	for _, n := range req.Diff.Nodes {
		valid[n.ID] = true
	}
	for _, ed := range req.Diff.Edges {
		valid[ed.ID] = true
	}
	out := e.Claims[:0]
	for _, c := range e.Claims {
		if strings.TrimSpace(c.Text) == "" {
			continue
		}
		ok := len(c.EvidenceIDs) > 0
		for _, id := range c.EvidenceIDs {
			if !valid[id] {
				ok = false
				break
			}
		}
		if ok {
			if c.Classification == "" {
				c.Classification = graph.Potential
			}
			out = append(out, c)
		}
	}
	e.Claims = out
	if strings.TrimSpace(e.Summary) == "" || strings.EqualFold(strings.TrimSpace(e.Summary), "brief factual summary") {
		return e, fmt.Errorf("empty explanation summary")
	}
	return e, nil
}
func Parse(raw string, req Request) (Explanation, error) {
	start, end := strings.Index(raw, "{"), strings.LastIndex(raw, "}")
	if start < 0 || end < start {
		return Explanation{}, fmt.Errorf("provider returned no JSON object")
	}
	var e Explanation
	if err := json.Unmarshal([]byte(raw[start:end+1]), &e); err != nil {
		return e, err
	}
	return Validate(e, req)
}
func Deterministic(req Request, reason string) Explanation {
	s := req.Diff.Summary
	summary := fmt.Sprintf("%d nodes added, %d removed, %d modified; %d relationships added and %d removed.", s.NodesAdded, s.NodesRemoved, s.NodesModified, s.EdgesAdded, s.EdgesRemoved)
	if reason != "" {
		summary += " AI explanation unavailable: " + reason
	}
	e := Explanation{Summary: summary, Provider: "deterministic", Fallback: true}
	ids := append([]string(nil), req.Impact.Changed...)
	sort.Strings(ids)
	if len(ids) > 0 {
		e.Claims = append(e.Claims, Claim{Text: fmt.Sprintf("%d changed symbols or components are supported by static evidence.", len(ids)), EvidenceIDs: ids, Classification: graph.Resolved})
	}
	return e
}
