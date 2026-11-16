// Package graph defines Impactr's deterministic, versioned fact graph.
package graph

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = "1.0.0"

type Change string

const (
	Unchanged Change = "unchanged"
	Added     Change = "added"
	Removed   Change = "removed"
	Modified  Change = "modified"
)

type Resolution string

const (
	Resolved   Resolution = "resolved"
	Unresolved Resolution = "unresolved"
	Potential  Resolution = "potential"
)

type Location struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}
type Evidence struct {
	Kind     string   `json:"kind"`
	Location Location `json:"location"`
	Detail   string   `json:"detail"`
}
type Node struct {
	ID            string     `json:"id"`
	Kind          string     `json:"kind"`
	Name          string     `json:"name"`
	QualifiedName string     `json:"qualified_name"`
	Language      string     `json:"language,omitempty"`
	Location      Location   `json:"location,omitempty"`
	Exported      bool       `json:"exported,omitempty"`
	Signature     string     `json:"signature,omitempty"`
	BodyHash      string     `json:"body_hash,omitempty"`
	Change        Change     `json:"change"`
	Resolution    Resolution `json:"resolution"`
	Evidence      []Evidence `json:"evidence,omitempty"`
}
type Edge struct {
	ID         string     `json:"id"`
	From       string     `json:"from"`
	To         string     `json:"to"`
	Kind       string     `json:"kind"`
	Change     Change     `json:"change"`
	Resolution Resolution `json:"resolution"`
	Evidence   []Evidence `json:"evidence,omitempty"`
}
type Provenance struct {
	BaseSHA        string            `json:"base_sha,omitempty"`
	HeadSHA        string            `json:"head_sha,omitempty"`
	Comparison     string            `json:"comparison"`
	GeneratedAt    time.Time         `json:"generated_at"`
	ImpactrVersion string            `json:"impactr_version"`
	Analyzers      map[string]string `json:"analyzers"`
}
type Graph struct {
	SchemaVersion string     `json:"schema_version"`
	Revision      string     `json:"revision"`
	Nodes         []Node     `json:"nodes"`
	Edges         []Edge     `json:"edges"`
	Warnings      []string   `json:"warnings,omitempty"`
	Provenance    Provenance `json:"provenance"`
}
type Diff struct {
	SchemaVersion string        `json:"schema_version"`
	Nodes         []Node        `json:"nodes"`
	Edges         []Edge        `json:"edges"`
	Summary       ChangeSummary `json:"summary"`
}
type ChangeSummary struct{ NodesAdded, NodesRemoved, NodesModified, EdgesAdded, EdgesRemoved int }
type Impact struct {
	Changed   []string   `json:"changed"`
	Affected  []Affected `json:"affected"`
	Truncated bool       `json:"truncated"`
}
type Affected struct {
	NodeID         string     `json:"node_id"`
	Depth          int        `json:"depth"`
	Via            []string   `json:"via"`
	Classification Resolution `json:"classification"`
}

func StableNodeID(language, pkg, qualified, kind string) string {
	return stableID("node", language, pkg, qualified, kind)
}
func StableEdgeID(from, to, kind string) string { return stableID("edge", from, to, kind) }
func stableID(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:12])
}
func Hash(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

func (g *Graph) Normalize() {
	g.SchemaVersion = SchemaVersion
	for i := range g.Nodes {
		if g.Nodes[i].Change == "" {
			g.Nodes[i].Change = Unchanged
		}
		if g.Nodes[i].Resolution == "" {
			g.Nodes[i].Resolution = Resolved
		}
	}
	for i := range g.Edges {
		if g.Edges[i].ID == "" {
			g.Edges[i].ID = StableEdgeID(g.Edges[i].From, g.Edges[i].To, g.Edges[i].Kind)
		}
		if g.Edges[i].Change == "" {
			g.Edges[i].Change = Unchanged
		}
		if g.Edges[i].Resolution == "" {
			g.Edges[i].Resolution = Resolved
		}
	}
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	sort.Slice(g.Edges, func(i, j int) bool { return g.Edges[i].ID < g.Edges[j].ID })
	sort.Strings(g.Warnings)
}
