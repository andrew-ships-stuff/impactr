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
	"math"
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

const (
	nodeW      = 176
	nodeH      = 56
	colGap     = 96
	rowGap     = 34
	margin     = 30
	legendH    = 46
	minCanvasW = 640
	minCanvasH = 220
	maxCanvasW = 1400
)

type placement struct {
	pos            map[string]point
	width, height  int
}

// layout places nodes in deterministic left-to-right dependency layers:
// every edge From→To points rightward. Ranking uses the longest path on a
// DFS-acyclic view; within a layer, node order is reduced for crossings with
// barycenter sweeps. All inputs are sorted, so output is stable.
func layout(d graph.Diff, max int) placement {
	if max <= 0 || max > len(d.Nodes) {
		max = len(d.Nodes)
	}
	sel := append([]graph.Node(nil), d.Nodes...)
	sort.Slice(sel, func(i, j int) bool { return sel[i].ID < sel[j].ID })
	sel = sel[:max]
	kept := make(map[string]bool, len(sel))
	ids := make([]string, 0, len(sel))
	for _, n := range sel {
		if kept[n.ID] {
			continue
		}
		kept[n.ID] = true
		ids = append(ids, n.ID)
	}
	sort.Strings(ids)
	succ := make(map[string][]string)
	pred := make(map[string][]string)
	seen := make(map[string]bool)
	for _, e := range d.Edges {
		if !kept[e.From] || !kept[e.To] || e.From == e.To {
			continue
		}
		k := e.From + "\x00" + e.To
		if seen[k] {
			continue
		}
		seen[k] = true
		succ[e.From] = append(succ[e.From], e.To)
		pred[e.To] = append(pred[e.To], e.From)
	}
	for _, v := range succ {
		sort.Strings(v)
	}
	for _, v := range pred {
		sort.Strings(v)
	}
	state := make(map[string]int, len(ids))
	dag := make(map[string][]string, len(ids))
	var visit func(string)
	visit = func(v string) {
		state[v] = 1
		for _, w := range succ[v] {
			if state[w] == 0 {
				visit(w)
			}
			if state[w] == 2 {
				dag[v] = append(dag[v], w)
			}
		}
		state[v] = 2
	}
	for _, id := range ids {
		if state[id] == 0 {
			visit(id)
		}
	}
	rank := make(map[string]int, len(ids))
	done := make(map[string]bool, len(ids))
	var rankOf func(string) int
	rankOf = func(v string) int {
		if done[v] {
			return rank[v]
		}
		done[v] = true
		best := 0
		for _, w := range dag[v] {
			if r := rankOf(w) + 1; r > best {
				best = r
			}
		}
		rank[v] = best
		return best
	}
	cols := 0
	for _, id := range ids {
		if r := rankOf(id) + 1; r > cols {
			cols = r
		}
	}
	layers := make([][]string, cols)
	for _, id := range ids {
		r := rank[id]
		layers[r] = append(layers[r], id)
	}
	for _, l := range layers {
		sort.Strings(l)
	}
	index := make(map[string]int, len(ids))
	reindex := func() {
		for _, l := range layers {
			for i, id := range l {
				index[id] = i
			}
		}
	}
	reindex()
	bary := func(id string, nb map[string][]string, target int) float64 {
		sum, count := 0.0, 0
		for _, w := range nb[id] {
			if rank[w] == target {
				sum += float64(index[w])
				count++
			}
		}
		if count == 0 {
			return float64(index[id])
		}
		return sum / float64(count)
	}
	for iter := 0; iter < 4; iter++ {
		if iter%2 == 0 {
			for l := 1; l < cols; l++ {
				sort.SliceStable(layers[l], func(i, j int) bool {
					a, b := layers[l][i], layers[l][j]
					ba, bb := bary(a, pred, l-1), bary(b, pred, l-1)
					if ba != bb {
						return ba < bb
					}
					return a < b
				})
			}
		} else {
			for l := cols - 2; l >= 0; l-- {
				sort.SliceStable(layers[l], func(i, j int) bool {
					a, b := layers[l][i], layers[l][j]
					ba, bb := bary(a, succ, l+1), bary(b, succ, l+1)
					if ba != bb {
						return ba < bb
					}
					return a < b
				})
			}
		}
		reindex()
	}
	rows := 0
	for _, l := range layers {
		if len(l) > rows {
			rows = len(l)
		}
	}
	gapX := colGap
	width := margin*2 + cols*nodeW + (cols-1)*gapX
	if width > maxCanvasW && cols > 1 {
		gapX = (maxCanvasW - margin*2 - cols*nodeW) / (cols - 1)
		if gapX < 16 {
			gapX = 16
		}
		width = margin*2 + cols*nodeW + (cols-1)*gapX
	}
	if width < minCanvasW {
		width = minCanvasW
	}
	height := margin*2 + rows*(nodeH+rowGap) - rowGap + legendH
	if height < minCanvasH {
		height = minCanvasH
	}
	pos := make(map[string]point, len(ids))
	blockW := cols*nodeW + (cols-1)*gapX
	x0 := (width - blockW) / 2
	blockH := rows*(nodeH+rowGap) - rowGap
	for l, layer := range layers {
		colH := len(layer)*(nodeH+rowGap) - rowGap
		y0 := margin + (blockH-colH)/2
		x := x0 + l*(nodeW+gapX)
		for i, id := range layer {
			pos[id] = point{x, y0 + i*(nodeH + rowGap)}
		}
	}
	return placement{pos: pos, width: width, height: height}
}

// exitPoint clips the center-to-center segment at the box boundary so edges
// start and end on the node outline.
func exitPoint(p, toward point) point {
	cx, cy := p.x+nodeW/2, p.y+nodeH/2
	dx, dy := float64(toward.x-p.x), float64(toward.y-p.y)
	if dx == 0 && dy == 0 {
		return point{cx, cy}
	}
	t := 1.0
	if dx != 0 {
		if s := float64(nodeW) / 2 / math.Abs(dx); s < t {
			t = s
		}
	}
	if dy != 0 {
		if s := float64(nodeH) / 2 / math.Abs(dy); s < t {
			t = s
		}
	}
	return point{int(math.Round(float64(cx) + dx*t)), int(math.Round(float64(cy) + dy*t))}
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

func edgeStroke(c graph.Change) string {
	switch c {
	case graph.Added:
		return "#16a34a"
	case graph.Removed:
		return "#dc2626"
	case graph.Modified:
		return "#d97706"
	default:
		return "#64748b"
	}
}

func markerID(c graph.Change) string {
	switch c {
	case graph.Added:
		return "a-added"
	case graph.Removed:
		return "a-removed"
	case graph.Modified:
		return "a-modified"
	default:
		return "a-neutral"
	}
}

func fit(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func SVG(w io.Writer, d graph.Diff, max int, dark bool) error {
	pl := layout(d, max)
	bg, fg, muted := "#ffffff", "#0f172a", "#475569"
	if dark {
		bg, fg, muted = "#0f172a", "#e2e8f0", "#94a3b8"
	}
	fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img"><title>Impactr architecture impact</title><rect width="100%%" height="100%%" fill="%s"/><defs>`, pl.width, pl.height, pl.width, pl.height, bg)
	for _, m := range []struct{ id, color string }{{"a-neutral", "#64748b"}, {"a-added", "#16a34a"}, {"a-removed", "#dc2626"}, {"a-modified", "#d97706"}} {
		fmt.Fprintf(w, `<marker id="%s" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8z" fill="%s"/></marker>`, m.id, m.color)
	}
	fmt.Fprint(w, `</defs>`)
	for _, e := range d.Edges {
		a, aok := pl.pos[e.From]
		b, bok := pl.pos[e.To]
		if !aok || !bok {
			continue
		}
		p1, p2 := exitPoint(a, b), exitPoint(b, a)
		dash := ""
		if e.Resolution != graph.Resolved {
			dash = ` stroke-dasharray="5 4"`
		}
		stroke := edgeStroke(e.Change)
		fmt.Fprintf(w, `<path d="M%d %d L%d %d" fill="none" stroke="%s" stroke-width="1.5"%s marker-end="url(#%s)"/>`, p1.x, p1.y, p2.x, p2.y, stroke, dash, markerID(e.Change))
		dx, dy := p2.x-p1.x, p2.y-p1.y
		if math.Hypot(float64(dx), float64(dy)) > 70 {
			lx, ly := (p1.x+p2.x)/2, (p1.y+p2.y)/2-4
			fmt.Fprintf(w, `<text x="%d" y="%d" fill="%s" stroke="%s" stroke-width="3" paint-order="stroke" font-family="system-ui,sans-serif" font-size="10" text-anchor="middle">%s</text>`, lx, ly, muted, bg, html.EscapeString(fit(e.Kind, 14)))
		}
	}
	for _, n := range d.Nodes {
		q, ok := pl.pos[n.ID]
		if !ok {
			continue
		}
		fill, stroke := palette(n.Change)
		dash := ""
		if n.Resolution != graph.Resolved {
			dash = ` stroke-dasharray="5 3"`
		}
		sub := n.Kind + " · " + string(n.Change)
		fmt.Fprintf(w, `<g><rect x="%d" y="%d" width="%d" height="%d" rx="8" fill="%s" stroke="%s" stroke-width="2"%s/><text x="%d" y="%d" fill="#0f172a" font-family="system-ui,sans-serif" font-size="13" font-weight="600">%s</text><text x="%d" y="%d" fill="#475569" font-family="system-ui,sans-serif" font-size="10">%s</text></g>`, q.x, q.y, nodeW, nodeH, fill, stroke, dash, q.x+10, q.y+22, html.EscapeString(fit(n.Name, 20)), q.x+10, q.y+40, html.EscapeString(fit(sub, 32)))
	}
	lx, ly := margin, pl.height-24
	fmt.Fprintf(w, `<g font-family="system-ui,sans-serif" font-size="11" fill="%s"><text x="%d" y="%d" font-weight="600">Legend</text>`, fg, lx, ly)
	x := lx + 56
	for _, item := range []struct{ label, fill, stroke string }{{"added", "#dcfce7", "#16a34a"}, {"removed", "#fee2e2", "#dc2626"}, {"modified", "#fef3c7", "#d97706"}, {"unchanged", "#f1f5f9", "#64748b"}} {
		fmt.Fprintf(w, `<rect x="%d" y="%d" width="14" height="10" rx="2" fill="%s" stroke="%s"/><text x="%d" y="%d">%s</text>`, x, ly-9, item.fill, item.stroke, x+19, ly, item.label)
		x += 19 + 7*len(item.label) + 22
	}
	fmt.Fprintf(w, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#64748b" stroke-dasharray="5 3"/><text x="%d" y="%d">unresolved / potentially affected</text></g></svg>`, x, ly-4, x+22, ly-4, x+28, ly)
	return nil
}

func PNG(w io.Writer, d graph.Diff, max int) error {
	pl := layout(d, max)
	im := image.NewRGBA(image.Rect(0, 0, pl.width, pl.height))
	draw.Draw(im, im.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
	ink := color.RGBA{100, 116, 139, 255}
	for _, e := range d.Edges {
		a, aok := pl.pos[e.From]
		b, bok := pl.pos[e.To]
		if !aok || !bok {
			continue
		}
		line(im, exitPoint(a, b), exitPoint(b, a), ink)
	}
	dr := font.Drawer{Dst: im, Src: image.NewUniform(color.RGBA{15, 23, 42, 255}), Face: basicfont.Face7x13}
	for _, n := range d.Nodes {
		q, ok := pl.pos[n.ID]
		if !ok {
			continue
		}
		fill := color.RGBA{241, 245, 249, 255}
		border := color.RGBA{100, 116, 139, 255}
		switch n.Change {
		case graph.Added:
			fill, border = color.RGBA{220, 252, 231, 255}, color.RGBA{22, 163, 74, 255}
		case graph.Removed:
			fill, border = color.RGBA{254, 226, 226, 255}, color.RGBA{220, 38, 38, 255}
		case graph.Modified:
			fill, border = color.RGBA{254, 243, 199, 255}, color.RGBA{217, 119, 6, 255}
		}
		draw.Draw(im, image.Rect(q.x, q.y, q.x+nodeW, q.y+nodeH), &image.Uniform{fill}, image.Point{}, draw.Src)
		rect(im, q.x, q.y, nodeW, nodeH, border)
		dr.Dot = fixed.P(q.x+8, q.y+22)
		dr.DrawString(fit(n.Name, 23))
		dr.Dot = fixed.P(q.x+8, q.y+40)
		dr.DrawString(fit(n.Kind+" · "+string(n.Change), 24))
	}
	dr.Dot = fixed.P(margin, im.Bounds().Dy()-20)
	dr.DrawString("Impactr: added=green removed=red modified=amber dashed=potential")
	return png.Encode(w, im)
}

func line(im *image.RGBA, a, b point, c color.RGBA) {
	dx, dy := abs(b.x-a.x), abs(b.y-a.y)
	sx, sy := 1, 1
	if a.x > b.x {
		sx = -1
	}
	if a.y > b.y {
		sy = -1
	}
	err := dx - dy
	for {
		if a.x >= 0 && a.y >= 0 && a.x < im.Bounds().Dx() && a.y < im.Bounds().Dy() {
			im.Set(a.x, a.y, c)
		}
		if a.x == b.x && a.y == b.y {
			return
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			a.x += sx
		}
		if e2 < dx {
			err += dx
			a.y += sy
		}
	}
}

func rect(im *image.RGBA, x, y, w, h int, c color.RGBA) {
	for i := x; i < x+w; i++ {
		line(im, point{i, y}, point{i, y + h - 1}, c)
	}
	for j := y; j < y+h; j++ {
		line(im, point{x, j}, point{x + w - 1, j}, c)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func Mermaid(w io.Writer, d graph.Diff, max int) error {
	pl := layout(d, max)
	fmt.Fprintln(w, "flowchart LR")
	for _, n := range d.Nodes {
		if _, ok := pl.pos[n.ID]; !ok {
			continue
		}
		fmt.Fprintf(w, "  n%s[%q]\n", n.ID, strings.ReplaceAll(n.Name, "\"", "'"))
	}
	for _, e := range d.Edges {
		if _, a := pl.pos[e.From]; !a {
			continue
		}
		if _, b := pl.pos[e.To]; !b {
			continue
		}
		fmt.Fprintf(w, "  n%s -->|%s| n%s\n", e.From, e.Kind, e.To)
	}
	return nil
}
func Terminal(w io.Writer, r Report) error {
	s := r.Diff.Summary
	fmt.Fprintf(w, "Impactr — %s comparison\n\nNodes: +%d -%d ~%d   Relationships: +%d -%d   Potentially affected: %d\n", r.After.Provenance.Comparison, s.NodesAdded, s.NodesRemoved, s.NodesModified, s.EdgesAdded, s.EdgesRemoved, len(r.Impact.Affected))
	shown, total := 0, 0
	for _, n := range r.Diff.Nodes {
		if n.Change != graph.Unchanged {
			total++
		}
	}
	for _, n := range r.Diff.Nodes {
		if n.Change != graph.Unchanged {
			if shown >= 30 {
				continue
			}
			fmt.Fprintf(w, "  %-8s %-12s %s (%s:%d)\n", n.Change, n.Kind, n.QualifiedName, n.Location.Path, n.Location.StartLine)
			shown++
		}
	}
	if total > shown {
		fmt.Fprintf(w, "  … %d more changed nodes; see .impactr/analysis.json\n", total-shown)
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
func Bytes(fn func(io.Writer) error) ([]byte, error) {
	var b bytes.Buffer
	e := fn(&b)
	return b.Bytes(), e
}
