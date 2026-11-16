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

const (
	maxPromptNodes    = 40
	maxPromptEdges    = 40
	maxPromptAffected = 30
	// maxFactsBytes keeps the prompt comfortably inside n_ctx: ~3 bytes per
	// token leaves >2.5k tokens of headroom in the default 4096 context.
	maxFactsBytes = 3000
)

// changeRank orders facts the model most benefits from seeing first.
func changeRank(c graph.Change) int {
	switch c {
	case graph.Modified:
		return 0
	case graph.Removed:
		return 1
	case graph.Added:
		return 2
	}
	return 3
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
		Counts       struct {
			NodesAdded, NodesRemoved, NodesModified, EdgesAdded, EdgesRemoved, AffectedTotal int
		} `json:"counts"`
		Truncated bool `json:"truncated,omitempty"`
	}
	var c compact
	c.Counts.NodesAdded = req.Diff.Summary.NodesAdded
	c.Counts.NodesRemoved = req.Diff.Summary.NodesRemoved
	c.Counts.NodesModified = req.Diff.Summary.NodesModified
	c.Counts.EdgesAdded = req.Diff.Summary.EdgesAdded
	c.Counts.EdgesRemoved = req.Diff.Summary.EdgesRemoved
	c.Counts.AffectedTotal = len(req.Impact.Affected)
	nodes := append([]graph.Node(nil), req.Diff.Nodes...)
	sort.SliceStable(nodes, func(i, j int) bool {
		a, b := nodes[i], nodes[j]
		if a.Change != b.Change {
			return changeRank(a.Change) < changeRank(b.Change)
		}
		ar, br := a.Location.Path != "", b.Location.Path != ""
		if ar != br {
			return ar
		}
		return a.ID < b.ID
	})
	for _, n := range nodes {
		if len(c.ChangedNodes) >= maxPromptNodes {
			c.Truncated = true
			break
		}
		if n.Change != graph.Unchanged {
			c.ChangedNodes = append(c.ChangedNodes, nodeFact{ID: n.ID, Kind: n.Kind, Name: n.QualifiedName, Path: n.Location.Path, Signature: n.Signature, Change: n.Change, Resolution: n.Resolution})
		}
	}
	for _, e := range req.Diff.Edges {
		if len(c.ChangedEdges) >= maxPromptEdges {
			c.Truncated = true
			break
		}
		if e.Change != graph.Unchanged {
			c.ChangedEdges = append(c.ChangedEdges, edgeFact{ID: e.ID, From: e.From, To: e.To, Kind: e.Kind, Change: e.Change, Resolution: e.Resolution})
		}
	}
	for _, a := range req.Impact.Affected {
		if len(c.Affected) >= maxPromptAffected {
			c.Truncated = true
			break
		}
		c.Affected = append(c.Affected, a)
	}
	b, _ := json.Marshal(c)
	for c.Truncated || len(b) > maxFactsBytes {
		c.Truncated = true
		if len(c.ChangedNodes) > len(c.Affected) && len(c.ChangedNodes) > 5 {
			c.ChangedNodes = c.ChangedNodes[:max(5, (len(c.ChangedNodes)*3)/4)]
		} else if len(c.Affected) > len(c.ChangedEdges) && len(c.Affected) > 3 {
			c.Affected = c.Affected[:max(3, (len(c.Affected)*3)/4)]
		} else if len(c.ChangedEdges) > 3 {
			c.ChangedEdges = c.ChangedEdges[:max(3, (len(c.ChangedEdges)*3)/4)]
		} else {
			break
		}
		b, _ = json.Marshal(c)
	}
	return b
}
func Prompt(req Request) string {
	return fmt.Sprintf(`Explain this architecture change using ONLY the JSON facts. The static counts are: %d nodes added, %d removed, %d modified, %d edges added, %d removed. Output one JSON object with keys "summary" and "claims". Summary must be a concrete sentence naming important changed symbols; never output instructions or placeholder phrases. Facts may be truncated; the totals in "counts" are authoritative. Each claim has keys "text", "evidence_ids", and "classification" (resolved, potential, or unresolved). Cite only IDs present in facts. Never invent dependencies. Reachable impact is potential, not confirmed. Facts: %s`, req.Diff.Summary.NodesAdded, req.Diff.Summary.NodesRemoved, req.Diff.Summary.NodesModified, req.Diff.Summary.EdgesAdded, req.Diff.Summary.EdgesRemoved, string(Facts(req)))
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
