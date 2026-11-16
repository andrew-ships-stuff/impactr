package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	gitx "github.com/andrew-ships-stuff/impactr/internal/git"
	"github.com/andrew-ships-stuff/impactr/internal/graph"
	"github.com/andrew-ships-stuff/impactr/internal/inference"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"html"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"sort"
	"strings"
)

type Report struct {
	SchemaVersion string                `json:"schema_version"`
	Before        graph.Graph           `json:"before"`
	After         graph.Graph           `json:"after"`
	Diff          graph.Diff            `json:"diff"`
	Impact        graph.Impact          `json:"impact"`
	Explanation   inference.Explanation `json:"explanation"`
	Files         []gitx.FileChange     `json:"files"`
}
type point struct{ x, y int }

func layout(nodes []graph.Node, max int) map[string]point {
	if max <= 0 || max > len(nodes) {
		max = len(nodes)
	}
	n := append([]graph.Node(nil), nodes...)
	sort.Slice(n, func(i, j int) bool { return n[i].ID < n[j].ID })
	used := map[int]bool{}
	out := map[string]point{}
	const cols = 4
	for _, v := range n[:max] {
		seed := 0
		for i := 0; i < len(v.ID) && i < 8; i++ {
			seed = seed*33 + int(v.ID[i])
		}
		slot := seed % (max*2 + 1)
		for used[slot] {
			slot++
		}
		used[slot] = true
		out[v.ID] = point{40 + (slot%cols)*190, 60 + (slot/cols)*100}
	}
	return out
}
func palette(c graph.Change) (fill, stroke string) {
	switch c {
	case graph.Added:
		return "#dcfce7", "#16a34a"
	case graph.Removed:
		return "#fee2e2", "#dc2626"
	case graph.Modified:
		return "#fef3c7", "#d97706"
	default:
		return "#f1f5f9", "#64748b"
	}
}
func SVG(w io.Writer, d graph.Diff, max int, dark bool) error {
	p := layout(d.Nodes, max)
	rows := (len(p) + 3) / 4
	height := maxi(220, rows*100+100)
	bg, fg := "#ffffff", "#0f172a"
	if dark {
		bg, fg = "#0f172a", "#e2e8f0"
	}
	fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" width="800" height="%d" viewBox="0 0 800 %d" role="img"><title>Impactr architecture impact</title><rect width="100%%" height="100%%" fill="%s"/><defs><marker id="a" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8z" fill="#64748b"/></marker></defs>`, height, height, bg)
	for _, e := range d.Edges {
		a, aok := p[e.From]
		b, bok := p[e.To]
		if !aok || !bok {
			continue
		}
		dash := ""
		if e.Resolution != graph.Resolved {
			dash = ` stroke-dasharray="5 4"`
		}
		fmt.Fprintf(w, `<path d="M%d %d L%d %d" fill="none" stroke="#64748b"%s marker-end="url(#a)"/><text x="%d" y="%d" fill="%s" font-size="10">%s</text>`, a.x+150, a.y+25, b.x, b.y+25, dash, (a.x+b.x)/2, (a.y+b.y)/2, fg, html.EscapeString(e.Kind))
	}
	for _, n := range d.Nodes {
		q, ok := p[n.ID]
		if !ok {
			continue
		}
		fill, stroke := palette(n.Change)
		dash := ""
		if n.Resolution != graph.Resolved {
			dash = ` stroke-dasharray="5 3"`
		}
		name := n.Name
		if len(name) > 22 {
			name = name[:21] + "…"
		}
		fmt.Fprintf(w, `<g><rect x="%d" y="%d" width="150" height="50" rx="8" fill="%s" stroke="%s" stroke-width="2"%s/><text x="%d" y="%d" fill="#0f172a" font-family="system-ui,sans-serif" font-size="13" font-weight="600">%s</text><text x="%d" y="%d" fill="#475569" font-family="system-ui,sans-serif" font-size="10">%s · %s</text></g>`, q.x, q.y, fill, stroke, dash, q.x+8, q.y+20, html.EscapeString(name), q.x+8, q.y+38, html.EscapeString(n.Kind), n.Change)
	}
	fmt.Fprintf(w, `<g transform="translate(20 %d)" font-family="system-ui" font-size="11" fill="%s"><text>Legend: 🟢 added  🔴 removed  🟠 modified  neutral unchanged  dashed unresolved/potential</text></g></svg>`, height-20, fg)
	return nil
}
func PNG(w io.Writer, d graph.Diff, max int) error {
	p := layout(d.Nodes, max)
	rows := (len(p) + 3) / 4
	im := image.NewRGBA(image.Rect(0, 0, 800, maxi(220, rows*100+100)))
	draw.Draw(im, im.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
	dr := font.Drawer{Dst: im, Src: image.NewUniform(color.RGBA{15, 23, 42, 255}), Face: basicfont.Face7x13}
	for _, n := range d.Nodes {
		q, ok := p[n.ID]
		if !ok {
			continue
		}
		fill := color.RGBA{241, 245, 249, 255}
		switch n.Change {
		case graph.Added:
			fill = color.RGBA{220, 252, 231, 255}
		case graph.Removed:
			fill = color.RGBA{254, 226, 226, 255}
		case graph.Modified:
			fill = color.RGBA{254, 243, 199, 255}
		}
		draw.Draw(im, image.Rect(q.x, q.y, q.x+150, q.y+50), &image.Uniform{fill}, image.Point{}, draw.Src)
		name := n.Name
		if len(name) > 20 {
			name = name[:20]
		}
		dr.Dot = fixed.P(q.x+6, q.y+20)
		dr.DrawString(name)
		dr.Dot = fixed.P(q.x+6, q.y+38)
		dr.DrawString(n.Kind + " · " + string(n.Change))
	}
	dr.Dot = fixed.P(20, im.Bounds().Dy()-20)
	dr.DrawString("Impactr: added=green removed=red modified=amber")
	return png.Encode(w, im)
}
func Mermaid(w io.Writer, d graph.Diff, max int) error {
	p := layout(d.Nodes, max)
	fmt.Fprintln(w, "flowchart LR")
	for _, n := range d.Nodes {
		if _, ok := p[n.ID]; !ok {
			continue
		}
		fmt.Fprintf(w, "  n%s[%q]\n", n.ID, strings.ReplaceAll(n.Name, "\"", "'"))
	}
	for _, e := range d.Edges {
		if _, a := p[e.From]; !a {
			continue
		}
		if _, b := p[e.To]; !b {
			continue
		}
		fmt.Fprintf(w, "  n%s -->|%s| n%s\n", e.From, e.Kind, e.To)
	}
	return nil
}
func Terminal(w io.Writer, r Report) error {
	s := r.Diff.Summary
	fmt.Fprintf(w, "Impactr — %s comparison\n\nNodes: +%d -%d ~%d   Relationships: +%d -%d   Potentially affected: %d\n", r.After.Provenance.Comparison, s.NodesAdded, s.NodesRemoved, s.NodesModified, s.EdgesAdded, s.EdgesRemoved, len(r.Impact.Affected))
	for _, n := range r.Diff.Nodes {
		if n.Change != graph.Unchanged {
			fmt.Fprintf(w, "  %-8s %-12s %s (%s:%d)\n", n.Change, n.Kind, n.QualifiedName, n.Location.Path, n.Location.StartLine)
		}
	}
	fmt.Fprintln(w, "\n"+r.Explanation.Summary)
	for _, wrg := range r.After.Warnings {
		fmt.Fprintln(w, "warning:", wrg)
	}
	return nil
}
func Markdown(r Report) string {
	var b strings.Builder
	s := r.Diff.Summary
	fmt.Fprintf(&b, "<!-- impactr-comment:v1 -->\n## Impactr architecture impact\n\n**+%d / −%d / ~%d nodes** · **+%d / −%d relationships** · **%d potentially affected**\n\n%s\n\n![Impact graph](impact.png)\n\n<details><summary>Evidence-backed findings</summary>\n\n", s.NodesAdded, s.NodesRemoved, s.NodesModified, s.EdgesAdded, s.EdgesRemoved, len(r.Impact.Affected), r.Explanation.Summary)
	for _, c := range r.Explanation.Claims {
		fmt.Fprintf(&b, "- %s (`%s`)\n", c.Text, c.Classification)
	}
	b.WriteString("\n</details>\n")
	return b.String()
}

func ImpactDiff(d graph.Diff, impact graph.Impact) graph.Diff {
	keep := map[string]bool{}
	for _, id := range impact.Changed {
		keep[id] = true
	}
	for _, a := range impact.Affected {
		keep[a.NodeID] = true
	}
	out := graph.Diff{SchemaVersion: d.SchemaVersion, Summary: d.Summary}
	for _, n := range d.Nodes {
		if keep[n.ID] {
			if n.Change == graph.Unchanged {
				n.Resolution = graph.Potential
			}
			out.Nodes = append(out.Nodes, n)
		}
	}
	for _, e := range d.Edges {
		if keep[e.From] && keep[e.To] {
			out.Edges = append(out.Edges, e)
		}
	}
	return out
}
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
func maxi(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func Bytes(fn func(io.Writer) error) ([]byte, error) {
	var b bytes.Buffer
	e := fn(&b)
	return b.Bytes(), e
}
