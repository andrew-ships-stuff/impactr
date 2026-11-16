package graph

import "sort"

func Compare(before, after Graph) Diff {
	bn, an := map[string]Node{}, map[string]Node{}
	be, ae := map[string]Edge{}, map[string]Edge{}
	for _, n := range before.Nodes {
		bn[n.ID] = n
	}
	for _, n := range after.Nodes {
		an[n.ID] = n
	}
	for _, e := range before.Edges {
		be[e.ID] = e
	}
	for _, e := range after.Edges {
		ae[e.ID] = e
	}
	d := Diff{SchemaVersion: SchemaVersion}
	ids := unionKeys(bn, an)
	for _, id := range ids {
		b, bok := bn[id]
		a, aok := an[id]
		switch {
		case !bok:
			a.Change = Added
			d.Summary.NodesAdded++
			d.Nodes = append(d.Nodes, a)
		case !aok:
			b.Change = Removed
			d.Summary.NodesRemoved++
			d.Nodes = append(d.Nodes, b)
		case nodeChanged(b, a):
			a.Change = Modified
			d.Summary.NodesModified++
			d.Nodes = append(d.Nodes, a)
		default:
			a.Change = Unchanged
			d.Nodes = append(d.Nodes, a)
		}
	}
	eids := unionKeys(be, ae)
	for _, id := range eids {
		b, bok := be[id]
		a, aok := ae[id]
		switch {
		case !bok:
			a.Change = Added
			d.Summary.EdgesAdded++
			d.Edges = append(d.Edges, a)
		case !aok:
			b.Change = Removed
			d.Summary.EdgesRemoved++
			d.Edges = append(d.Edges, b)
		default:
			a.Change = Unchanged
			d.Edges = append(d.Edges, a)
		}
	}
	return d
}
func nodeChanged(a, b Node) bool {
	return a.Signature != b.Signature || a.BodyHash != b.BodyHash || a.Exported != b.Exported || a.Kind != b.Kind
}
func unionKeys[T any](a, b map[string]T) []string {
	m := map[string]struct{}{}
	for k := range a {
		m[k] = struct{}{}
	}
	for k := range b {
		m[k] = struct{}{}
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func BlastRadius(d Diff, depth, budget int) Impact {
	if depth < 1 {
		depth = 1
	}
	if budget < 1 {
		budget = 250
	}
	changed := map[string]bool{}
	var q []Affected
	for _, n := range d.Nodes {
		if n.Change == Added || n.Change == Removed || n.Change == Modified {
			changed[n.ID] = true
			q = append(q, Affected{NodeID: n.ID})
		}
	}
	result := Impact{}
	for id := range changed {
		result.Changed = append(result.Changed, id)
	}
	sort.Strings(result.Changed)
	rev := map[string][]Edge{}
	for _, e := range d.Edges {
		if e.Change != Removed {
			rev[e.To] = append(rev[e.To], e)
		}
	}
	seen := map[string]bool{}
	for id := range changed {
		seen[id] = true
	}
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		if cur.Depth >= depth {
			continue
		}
		edges := rev[cur.NodeID]
		sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })
		for _, e := range edges {
			if seen[e.From] {
				continue
			}
			if len(result.Affected) >= budget {
				result.Truncated = true
				return result
			}
			seen[e.From] = true
			cls := Potential
			if e.Resolution == Unresolved {
				cls = Unresolved
			}
			a := Affected{NodeID: e.From, Depth: cur.Depth + 1, Via: []string{e.ID}, Classification: cls}
			result.Affected = append(result.Affected, a)
			q = append(q, a)
		}
	}
	sort.Slice(result.Affected, func(i, j int) bool {
		if result.Affected[i].Depth != result.Affected[j].Depth {
			return result.Affected[i].Depth < result.Affected[j].Depth
		}
		return result.Affected[i].NodeID < result.Affected[j].NodeID
	})
	return result
}
