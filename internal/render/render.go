package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	gitx "github.com/andrew-ships-stuff/impactr/internal/git"
	"github.com/andrew-ships-stuff/impactr/internal/graph"
	"github.com/andrew-ships-stuff/impactr/internal/inference"
	"golang.org/x/image/font"
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
	"sync"

	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
)

type Image struct {
	Path    string `json:"path"`
	Caption string `json:"caption"`
}

type Report struct {
	SchemaVersion string                `json:"schema_version"`
	Before        graph.Graph           `json:"before"`
	After         graph.Graph           `json:"after"`
	Diff          graph.Diff            `json:"diff"`
	Impact        graph.Impact          `json:"impact"`
	Explanation   inference.Explanation `json:"explanation"`
	Files         []gitx.FileChange     `json:"files"`
	Brief         *inference.Brief      `json:"brief,omitempty"`
	Rollup        []graph.PackageView   `json:"rollup,omitempty"`
	Images        []Image               `json:"images,omitempty"`
}

type point struct{ x, y int }

const (
	nodeW      = 208
	nodeH      = 68
	colGap     = 112
	rowGap     = 30
	margin     = 40
	legendH    = 58
	minCanvasW = 720
	minCanvasH = 260
	maxCanvasW = 1400
)

type placement struct {
	pos           map[string]point
	width, height int
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
		// Rank zero is a dependency. Reverse the rank coordinate so From→To
		// reads naturally from left to right.
		x := x0 + (cols-1-l)*(nodeW+gapX)
		for i, id := range layer {
			pos[id] = point{x, y0 + i*(nodeH+rowGap)}
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
	return SVGTitled(w, d, max, dark, "Impactr architecture impact")
}

func SVGTitled(w io.Writer, d graph.Diff, max int, dark bool, title string) error {
	pl := layout(d, max)
	bg, card, fg, muted, edge := "#f8fafc", "#ffffff", "#0f172a", "#64748b", "#cbd5e1"
	if dark {
		bg, card, fg, muted, edge = "#0b1220", "#111c2e", "#e5edf8", "#94a3b8", "#40516b"
	}
	fmt.Fprintf(w, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img"><title>%s</title><rect width="100%%" height="100%%" rx="14" fill="%s"/><defs><filter id="shadow" x="-20%%" y="-20%%" width="140%%" height="150%%"><feDropShadow dx="0" dy="2" stdDeviation="3" flood-color="#0f172a" flood-opacity=".12"/></filter>`, pl.width, pl.height, pl.width, pl.height, html.EscapeString(title), bg)
	for _, m := range []struct{ id, color string }{{"a-neutral", "#64748b"}, {"a-added", "#16a34a"}, {"a-removed", "#dc2626"}, {"a-modified", "#d97706"}} {
		fmt.Fprintf(w, `<marker id="%s" markerWidth="7" markerHeight="7" refX="6" refY="3.5" orient="auto"><path d="M0,0 L7,3.5 L0,7z" fill="%s"/></marker>`, m.id, m.color)
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
		stroke := edge
		if e.Change != graph.Unchanged {
			stroke = edgeStroke(e.Change)
		}
		bend := abs(p2.x-p1.x) / 2
		if bend < 28 {
			bend = 28
		}
		fmt.Fprintf(w, `<path d="M%d %d C%d %d %d %d %d %d" fill="none" stroke="%s" stroke-width="1.5" stroke-linecap="round" opacity=".85"%s marker-end="url(#%s)"/>`, p1.x, p1.y, p1.x+bend, p1.y, p2.x-bend, p2.y, p2.x, p2.y, stroke, dash, markerID(e.Change))
		dx, dy := p2.x-p1.x, p2.y-p1.y
		if e.Kind != "depends on" && math.Hypot(float64(dx), float64(dy)) > 100 {
			lx, ly := (p1.x+p2.x)/2, (p1.y+p2.y)/2-4
			fmt.Fprintf(w, `<text x="%d" y="%d" fill="%s" stroke="%s" stroke-width="4" paint-order="stroke" font-family="Inter,ui-sans-serif,system-ui,sans-serif" font-size="10" text-anchor="middle">%s</text>`, lx, ly, muted, bg, html.EscapeString(fit(e.Kind, 18)))
		}
	}
	for _, n := range d.Nodes {
		q, ok := pl.pos[n.ID]
		if !ok {
			continue
		}
		_, stroke := palette(n.Change)
		dash := ""
		if n.Resolution != graph.Resolved {
			dash = ` stroke-dasharray="5 3"`
		}
		status := string(n.Change)
		if n.Change == graph.Unchanged && n.Resolution != graph.Resolved {
			status = "potential impact"
		}
		fmt.Fprintf(w, `<g filter="url(#shadow)"><rect x="%d" y="%d" width="%d" height="%d" rx="11" fill="%s" stroke="%s" stroke-width="1"%s/><path d="M%d %d V%d" stroke="%s" stroke-width="4" stroke-linecap="round"/><text x="%d" y="%d" fill="%s" font-family="Inter,ui-sans-serif,system-ui,sans-serif" font-size="14" font-weight="650">%s</text><text x="%d" y="%d" fill="%s" font-family="Inter,ui-sans-serif,system-ui,sans-serif" font-size="10.5">%s</text><text x="%d" y="%d" fill="%s" font-family="Inter,ui-sans-serif,system-ui,sans-serif" font-size="9" font-weight="600" text-anchor="end" letter-spacing=".3">%s</text></g>`,
			q.x, q.y, nodeW, nodeH, card, edge, dash, q.x+3, q.y+12, q.y+nodeH-12, stroke,
			q.x+16, q.y+27, fg, html.EscapeString(fit(n.Name, 24)), q.x+16, q.y+48, muted, html.EscapeString(fit(n.Kind, 34)), q.x+nodeW-12, q.y+58, stroke, html.EscapeString(strings.ToUpper(status)))
	}
	lx, ly := margin, pl.height-24
	fmt.Fprintf(w, `<g font-family="Inter,ui-sans-serif,system-ui,sans-serif" font-size="10.5" fill="%s"><text x="%d" y="%d" font-weight="650">STATUS</text>`, muted, lx, ly)
	x := lx + 56
	for _, item := range []struct{ label, stroke string }{{"added", "#16a34a"}, {"removed", "#dc2626"}, {"modified", "#d97706"}, {"unchanged", "#64748b"}} {
		fmt.Fprintf(w, `<rect x="%d" y="%d" width="14" height="10" rx="3" fill="%s"/><text x="%d" y="%d">%s</text>`, x, ly-9, item.stroke, x+19, ly, item.label)
		x += 19 + 7*len(item.label) + 22
	}
	fmt.Fprintf(w, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#64748b" stroke-dasharray="5 3"/><text x="%d" y="%d">potential / unresolved</text></g></svg>`, x, ly-4, x+22, ly-4, x+28, ly)
	return nil
}

func PNG(w io.Writer, d graph.Diff, max int) error {
	pl := layout(d, max)
	im := image.NewRGBA(image.Rect(0, 0, pl.width, pl.height))
	draw.Draw(im, im.Bounds(), &image.Uniform{color.RGBA{248, 250, 252, 255}}, image.Point{}, draw.Src)
	ink := color.RGBA{203, 213, 225, 255}
	for _, e := range d.Edges {
		a, aok := pl.pos[e.From]
		b, bok := pl.pos[e.To]
		if !aok || !bok {
			continue
		}
		p1, p2 := exitPoint(a, b), exitPoint(b, a)
		line(im, p1, p2, ink)
		arrow(im, p1, p2, ink)
	}
	regular, err := openTypeFace(13)
	if err != nil {
		return err
	}
	defer regular.Close()
	small, err := openTypeFace(10)
	if err != nil {
		return err
	}
	defer small.Close()
	dr := font.Drawer{Dst: im, Src: image.NewUniform(color.RGBA{15, 23, 42, 255}), Face: regular}
	for _, n := range d.Nodes {
		q, ok := pl.pos[n.ID]
		if !ok {
			continue
		}
		accent := color.RGBA{100, 116, 139, 255}
		switch n.Change {
		case graph.Added:
			accent = color.RGBA{22, 163, 74, 255}
		case graph.Removed:
			accent = color.RGBA{220, 38, 38, 255}
		case graph.Modified:
			accent = color.RGBA{217, 119, 6, 255}
		}
		roundedRect(im, image.Rect(q.x+1, q.y+3, q.x+nodeW+1, q.y+nodeH+3), 11, color.RGBA{226, 232, 240, 255})
		roundedRect(im, image.Rect(q.x, q.y, q.x+nodeW, q.y+nodeH), 11, color.RGBA{203, 213, 225, 255})
		roundedRect(im, image.Rect(q.x+1, q.y+1, q.x+nodeW-1, q.y+nodeH-1), 10, color.RGBA{255, 255, 255, 255})
		for yy := q.y + 12; yy < q.y+nodeH-12; yy++ {
			for xx := q.x + 2; xx < q.x+6; xx++ {
				im.SetRGBA(xx, yy, accent)
			}
		}
		dr.Face = regular
		dr.Src = image.NewUniform(color.RGBA{15, 23, 42, 255})
		dr.Dot = fixed.P(q.x+16, q.y+27)
		dr.DrawString(fit(n.Name, 25))
		dr.Face = small
		dr.Src = image.NewUniform(color.RGBA{100, 116, 139, 255})
		dr.Dot = fixed.P(q.x+16, q.y+49)
		dr.DrawString(fit(n.Kind, 34))
	}
	dr.Face = small
	dr.Src = image.NewUniform(color.RGBA{71, 85, 105, 255})
	dr.Dot = fixed.P(margin, im.Bounds().Dy()-22)
	dr.DrawString("Impactr  ·  green added  ·  red removed  ·  amber modified  ·  gray potential")
	return png.Encode(w, im)
}

var (
	fontOnce sync.Once
	goFont   *opentype.Font
	fontErr  error
)

func openTypeFace(size float64) (font.Face, error) {
	fontOnce.Do(func() { goFont, fontErr = opentype.Parse(goregular.TTF) })
	if fontErr != nil {
		return nil, fontErr
	}
	return opentype.NewFace(goFont, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
}

func roundedRect(im *image.RGBA, r image.Rectangle, radius int, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			dx := max(r.Min.X+radius-x, x-(r.Max.X-radius-1), 0)
			dy := max(r.Min.Y+radius-y, y-(r.Max.Y-radius-1), 0)
			if dx*dx+dy*dy <= radius*radius && image.Pt(x, y).In(im.Bounds()) {
				im.SetRGBA(x, y, c)
			}
		}
	}
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

func arrow(im *image.RGBA, from, to point, c color.RGBA) {
	dx, dy := float64(to.x-from.x), float64(to.y-from.y)
	length := math.Hypot(dx, dy)
	if length == 0 {
		return
	}
	ux, uy := dx/length, dy/length
	left := point{int(float64(to.x) - 8*ux + 4*uy), int(float64(to.y) - 8*uy - 4*ux)}
	right := point{int(float64(to.x) - 8*ux - 4*uy), int(float64(to.y) - 8*uy + 4*ux)}
	line(im, to, left, c)
	line(im, to, right, c)
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
	headline := r.Explanation.Summary
	if r.Brief != nil && strings.TrimSpace(r.Brief.Headline) != "" {
		headline = r.Brief.Headline
	}
	changed := s.NodesAdded + s.NodesRemoved + s.NodesModified
	localChanged, packages, dependencies := 0, 0, 0
	for _, v := range r.Rollup {
		if v.External {
			dependencies += v.Added + v.Removed + v.Modified
		} else if v.Added+v.Removed+v.Modified > 0 {
			packages++
			localChanged += v.Added + v.Removed + v.Modified
		}
	}
	direct := 0
	for _, a := range r.Impact.Affected {
		if a.Depth <= 1 {
			direct++
		}
	}
	fmt.Fprintf(&b, "<!-- impactr-comment:v1 -->\n<!-- impactr-stats:v1 nodes=%d edges=%d affected=%d head=%s -->\n## Impactr · architecture impact\n\n> %s\n\n| | What the static analysis found |\n|---|---|\n| **Change scope** | %d %s across %d local %s |\n| **Blast radius** | %d potential %s: %d direct, %d transitive |\n| **Relationships** | %d added, %d removed |\n",
		changed, s.EdgesAdded+s.EdgesRemoved, len(r.Impact.Affected), r.After.Provenance.HeadSHA,
		safeMarkdownText(headline), localChanged, plural(localChanged, "symbol"), packages, plural(packages, "package"), len(r.Impact.Affected), plural(len(r.Impact.Affected), "dependent"), direct, len(r.Impact.Affected)-direct, s.EdgesAdded, s.EdgesRemoved)
	if dependencies > 0 {
		fmt.Fprintf(&b, "| **Dependencies** | %d external import relationships changed |\n", dependencies)
	}
	b.WriteString("\n")
	if len(r.Rollup) > 0 {
		b.WriteString("### Review first\n\n")
		top := append([]graph.PackageView(nil), r.Rollup...)
		sort.SliceStable(top, func(i, j int) bool {
			if top[i].External != top[j].External {
				return !top[i].External
			}
			if top[i].Affected != top[j].Affected {
				return top[i].Affected > top[j].Affected
			}
			return top[i].Added+top[i].Removed+top[i].Modified > top[j].Added+top[j].Removed+top[j].Modified
		})
		shown := 0
		for _, v := range top {
			if v.External || shown >= 6 {
				continue
			}
			count := v.Added + v.Removed + v.Modified
			if count == 0 && v.Affected == 0 {
				continue
			}
			pkg := strings.ReplaceAll(v.Package, "`", "'")
			if v.Affected > 0 {
				fmt.Fprintf(&b, "- **`%s`** — %d %s changed; **%d downstream %s may rely on them**.\n", pkg, count, plural(count, "symbol"), v.Affected, plural(v.Affected, "symbol"))
			} else {
				fmt.Fprintf(&b, "- **`%s`** — %d %s changed; no static dependents were found within the configured depth.\n", pkg, count, plural(count, "symbol"))
			}
			shown++
		}
		b.WriteString("\n")
	}
	if len(r.Impact.Affected) > 0 {
		byID := make(map[string]graph.Node, len(r.Diff.Nodes))
		for _, n := range r.Diff.Nodes {
			byID[n.ID] = n
		}
		b.WriteString("### Potential blast radius\n\n")
		for i, a := range r.Impact.Affected {
			if i >= 8 {
				fmt.Fprintf(&b, "- …and %d more in the downloadable analysis.\n", len(r.Impact.Affected)-i)
				break
			}
			n := byID[a.NodeID]
			name := n.QualifiedName
			if name == "" {
				name = n.Name
			}
			if name == "" {
				name = a.NodeID
			}
			distance := "direct dependent"
			if a.Depth > 1 {
				distance = fmt.Sprintf("transitive dependent, depth %d", a.Depth)
			}
			fmt.Fprintf(&b, "- `%s` — %s; **potential**, not a confirmed behavior change.\n", strings.ReplaceAll(name, "`", "'"), distance)
		}
		b.WriteString("\n")
	}
	if len(r.Images) > 0 {
		im := r.Images[0]
		fmt.Fprintf(&b, "![%s](%s)\n\n", safeAlt(im.Caption), im.Path)
	}
	if len(r.Images) > 1 {
		b.WriteString("<details><summary>Focused package flows</summary>\n\n")
		for _, im := range r.Images[1:] {
			fmt.Fprintf(&b, "**%s**\n\n![%s](%s)\n\n", safeMarkdownText(im.Caption), safeAlt(im.Caption), im.Path)
		}
		b.WriteString("</details>\n\n")
	}
	if len(r.Explanation.Claims) > 0 {
		b.WriteString("<details><summary>Evidence-backed AI explanation</summary>\n\n")
		for _, c := range r.Explanation.Claims {
			fmt.Fprintf(&b, "- %s (`%s`)\n", safeMarkdownText(c.Text), c.Classification)
		}
		b.WriteString("\n</details>\n")
	}
	return b.String()
}

func safeMarkdownText(s string) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\r", " ")
	s = strings.ReplaceAll(s, "<", "&lt;")
	return strings.ReplaceAll(s, "\n", " ")
}

func plural(n int, singular string) string {
	if n == 1 {
		return singular
	}
	return singular + "s"
}

func safeAlt(s string) string {
	s = safeMarkdownText(s)
	s = strings.ReplaceAll(s, "[", "(")
	return strings.ReplaceAll(s, "]", ")")
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
