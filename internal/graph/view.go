package graph

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// PackageName derives a deterministic package grouping for a node. Symbol
// and package nodes group by source directory (package nodes carry the
// package directory as their location path, which unifies them with their
// symbols); dependencies group by import path; everything else falls back to
// the qualified-name prefix.
func PackageName(n Node) string {
	if n.Kind == "external_dependency" && n.Name != "" {
		if n.QualifiedName != "" {
			return n.QualifiedName
		}
		return n.Name
	}
	if p := n.Location.Path; p != "" {
		if n.Kind == "package" || n.Kind == "module" {
			return filepath.ToSlash(p)
		}
		if d := filepath.Dir(p); d != "." && d != "/" {
			return d
		}
		return p
	}
	if q := n.QualifiedName; q != "" {
		if i := strings.LastIndexByte(q, '.'); i > 0 {
			return q[:i]
		}
		return q
	}
	return "(unknown)"
}

// PackageView aggregates impact facts at package granularity so comments and
// diagrams can stay readable when a diff touches hundreds of symbols.
type PackageView struct {
	Package  string   `json:"package"`
	Added    int      `json:"added"`
	Removed  int      `json:"removed"`
	Modified int      `json:"modified"`
	Affected int      `json:"affected"`
	Blast    int      `json:"blast"`
	External bool     `json:"external,omitempty"`
	NodeIDs  []string `json:"node_ids,omitempty"`
}

// PackageViews returns involved packages (changed or potentially affected),
// ranked by changed count, then reverse-dependency blast, then affected
// count, name last.
func PackageViews(d Diff, impact Impact) []PackageView {
	byID := make(map[string]Node, len(d.Nodes))
	for _, n := range d.Nodes {
		byID[n.ID] = n
	}
	views := map[string]*PackageView{}
	view := func(p string) *PackageView {
		if v, ok := views[p]; ok {
			return v
		}
		v := &PackageView{Package: p}
		views[p] = v
		return v
	}
	changed := map[string]bool{}
	for _, id := range impact.Changed {
		if n, ok := byID[id]; ok && n.Change != Unchanged {
			changed[id] = true
		}
	}
	callers := make(map[string][]string)
	for _, e := range d.Edges {
		if e.Change != Removed {
			callers[e.To] = append(callers[e.To], e.From)
		}
	}
	for id := range callers {
		sort.Strings(callers[id])
	}
	for id := range changed {
		n := byID[id]
		v := view(PackageName(n))
		switch n.Change {
		case Added:
			v.Added++
		case Removed:
			v.Removed++
		case Modified:
			v.Modified++
		}
		v.NodeIDs = append(v.NodeIDs, id)
	}
	allowed := map[string]bool{}
	for _, a := range impact.Affected {
		allowed[a.NodeID] = true
	}
	for _, a := range impact.Affected {
		n, ok := byID[a.NodeID]
		if !ok {
			continue
		}
		view(PackageName(n)).Affected++
	}
	changedByPackage := map[string][]string{}
	for id := range changed {
		p := PackageName(byID[id])
		changedByPackage[p] = append(changedByPackage[p], id)
	}
	for p, roots := range changedByPackage {
		view(p).Blast = len(reachableCallers(roots, callers, allowed))
	}
	out := make([]PackageView, 0, len(views))
	for _, v := range views {
		sort.Strings(v.NodeIDs)
		v.External = len(v.NodeIDs) > 0
		for _, id := range v.NodeIDs {
			if byID[id].Location.Path != "" {
				v.External = false
				break
			}
		}
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		ci, cj := out[i].Added+out[i].Removed+out[i].Modified, out[j].Added+out[j].Removed+out[j].Modified
		if ci != cj {
			return ci > cj
		}
		if out[i].Blast != out[j].Blast {
			return out[i].Blast > out[j].Blast
		}
		if out[i].Affected != out[j].Affected {
			return out[i].Affected > out[j].Affected
		}
		return out[i].Package < out[j].Package
	})
	return out
}

// Cluster is one change cluster: the changed symbols of a package plus the
// reverse-reachable potentially affected symbols, as an induced sub-diff
// suitable for a focused data-flow diagram.
type Cluster struct {
	Package  string   `json:"package"`
	Changed  []string `json:"changed"`
	Affected []string `json:"affected"`
	Diff     Diff     `json:"diff"`
}

// Clusters returns up to maxClusters data-flow views, ranked by blast radius,
// each capped at maxNodes nodes.
func Clusters(d Diff, impact Impact, maxClusters, maxNodes int) []Cluster {
	if maxClusters <= 0 {
		maxClusters = 4
	}
	if maxNodes <= 0 {
		maxNodes = 14
	}
	byID := make(map[string]Node, len(d.Nodes))
	for _, n := range d.Nodes {
		byID[n.ID] = n
	}
	depth := make(map[string]int, len(impact.Affected))
	for _, a := range impact.Affected {
		depth[a.NodeID] = a.Depth
	}
	pkgs := map[string][]string{}
	for _, id := range impact.Changed {
		if n, ok := byID[id]; ok && n.Change != Unchanged && n.Location.Path != "" {
			p := PackageName(n)
			pkgs[p] = append(pkgs[p], id)
		}
	}
	callersOf := make(map[string][]string)
	for _, e := range d.Edges {
		callersOf[e.To] = append(callersOf[e.To], e.From)
	}
	names := make([]string, 0, len(pkgs))
	for p := range pkgs {
		names = append(names, p)
	}
	allowed := map[string]bool{}
	for _, a := range impact.Affected {
		allowed[a.NodeID] = true
	}
	reach := func(p string) int { return len(reachableCallers(pkgs[p], callersOf, allowed)) }
	sort.Slice(names, func(i, j int) bool {
		si := 2*len(pkgs[names[i]]) + reach(names[i])
		sj := 2*len(pkgs[names[j]]) + reach(names[j])
		if si != sj {
			return si > sj
		}
		return names[i] < names[j]
	})
	if len(names) > maxClusters {
		names = names[:maxClusters]
	}
	out := make([]Cluster, 0, len(names))
	for _, p := range names {
		members := map[string]bool{}
		changed := append([]string(nil), pkgs[p]...)
		sort.Strings(changed)
		for _, id := range changed {
			if len(members) >= maxNodes {
				break
			}
			members[id] = true
		}
		affected := reachableCallers(changed, callersOf, allowed)
		sort.SliceStable(affected, func(i, j int) bool {
			if depth[affected[i]] != depth[affected[j]] {
				return depth[affected[i]] < depth[affected[j]]
			}
			return affected[i] < affected[j]
		})
		var aff []string
		for _, id := range affected {
			if len(members) >= maxNodes {
				break
			}
			if !members[id] {
				members[id] = true
				aff = append(aff, id)
			}
		}
		var cd Cluster
		cd.Package = p
		cd.Changed = changed
		cd.Affected = aff
		for _, n := range d.Nodes {
			if members[n.ID] {
				cd.Diff.Nodes = append(cd.Diff.Nodes, n)
			}
		}
		for _, e := range d.Edges {
			if members[e.From] && members[e.To] {
				cd.Diff.Edges = append(cd.Diff.Edges, e)
			}
		}
		out = append(out, cd)
	}
	return out
}

func reachableCallers(roots []string, callersOf map[string][]string, allowed map[string]bool) []string {
	seen := map[string]bool{}
	q := append([]string(nil), roots...)
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		for _, id := range callersOf[cur] {
			if !allowed[id] || seen[id] {
				continue
			}
			seen[id] = true
			q = append(q, id)
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// OverviewDiff projects changed and affected facts onto package-sized nodes
// for a compact top-level diagram.
func OverviewDiff(d Diff, impact Impact, max int) Diff {
	views := PackageViews(d, impact)
	// Standard-library and third-party imports remain in the report, but they
	// overwhelm the top-level picture when mixed with the repository's own
	// packages. The overview is for local architecture; dependency changes are
	// still visible in the rollup and focused graphs.
	local := make([]PackageView, 0, len(views))
	for _, v := range views {
		if !v.External {
			local = append(local, v)
		}
	}
	if len(local) > 1 {
		views = local
	}
	if max > 0 && len(views) > max {
		views = views[:max]
	}
	kept := map[string]bool{}
	for _, v := range views {
		kept[v.Package] = true
	}
	byID := make(map[string]Node, len(d.Nodes))
	for _, n := range d.Nodes {
		byID[n.ID] = n
	}
	nodeOf := map[string]*Node{}
	od := Diff{SchemaVersion: SchemaVersion, Summary: d.Summary}
	for _, v := range views {
		change := Unchanged
		switch {
		case v.Removed > 0 && v.Removed >= v.Added && v.Removed >= v.Modified:
			change = Removed
		case v.Added > 0 && v.Added >= v.Modified:
			change = Added
		case v.Modified > 0:
			change = Modified
		}
		res := Resolved
		if v.Affected > 0 && v.Added+v.Removed+v.Modified == 0 {
			res = Potential
		}
		n := Node{ID: stableID("pkgview", v.Package), Kind: fmt.Sprintf("package · %d changed · %d potential", v.Added+v.Removed+v.Modified, v.Affected), Name: v.Package, QualifiedName: v.Package, Change: change, Resolution: res}
		nodeOf[v.Package] = &n
		od.Nodes = append(od.Nodes, n)
	}
	counts := map[string]map[string]int{}
	for _, e := range d.Edges {
		f, t := byID[e.From], byID[e.To]
		fp, tp := PackageName(f), PackageName(t)
		if fp == tp || !kept[fp] || !kept[tp] {
			continue
		}
		if counts[fp] == nil {
			counts[fp] = map[string]int{}
		}
		counts[fp][tp]++
	}
	var es []Edge
	strength := map[string]int{}
	for fp, m := range counts {
		for tp := range m {
			from, to := nodeOf[fp].ID, nodeOf[tp].ID
			id := StableEdgeID(from, to, "depends on")
			strength[id] = m[tp]
			es = append(es, Edge{ID: id, From: from, To: to, Kind: "depends on", Resolution: Resolved, Change: Unchanged})
		}
	}
	sort.Slice(es, func(i, j int) bool {
		if es[i].From != es[j].From {
			return es[i].From < es[j].From
		}
		if strength[es[i].ID] != strength[es[j].ID] {
			return strength[es[i].ID] > strength[es[j].ID]
		}
		return es[i].To < es[j].To
	})
	// Dense package graphs quickly become a hairball. Retain a deterministic
	// two strongest outgoing relationships per package; focused cluster views
	// carry the symbol-level detail.
	if len(es) > 0 {
		limited := make([]Edge, 0, len(es))
		outgoing := map[string]int{}
		for _, e := range es {
			if outgoing[e.From] >= 2 {
				continue
			}
			outgoing[e.From]++
			limited = append(limited, e)
		}
		es = limited
		sort.Slice(es, func(i, j int) bool {
			if es[i].From != es[j].From {
				return es[i].From < es[j].From
			}
			return es[i].To < es[j].To
		})
	}
	od.Edges = es
	return od
}
