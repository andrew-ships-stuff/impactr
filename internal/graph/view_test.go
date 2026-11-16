package graph

import "testing"

func fixture() (Diff, Impact) {
	nodes := []Node{
		{ID: "v", Kind: "function", Name: "Validate", QualifiedName: "app/session.Validate", Location: Location{Path: "session/validator.go"}, Change: Added, Resolution: Resolved},
		{ID: "h", Kind: "function", Name: "Serve", QualifiedName: "app/handler.Serve", Location: Location{Path: "handler/user.go"}, Change: Unchanged, Resolution: Resolved},
		{ID: "m", Kind: "function", Name: "Main", QualifiedName: "app.Main", Location: Location{Path: "main.go"}, Change: Modified, Resolution: Resolved},
		{ID: "r", Kind: "function", Name: "Exists", QualifiedName: "app/storage.Exists", Location: Location{Path: "storage/redis.go"}, Change: Unchanged, Resolution: Resolved},
	}
	edges := []Edge{
		{ID: "e1", From: "h", To: "v", Kind: "calls"},
		{ID: "e2", From: "m", To: "h", Kind: "calls"},
		{ID: "e3", From: "r", To: "h", Kind: "calls"},
	}
	d := Diff{Nodes: nodes, Edges: edges}
	i := Impact{Changed: []string{"v", "m"}, Affected: []Affected{{NodeID: "h", Depth: 1}, {NodeID: "r", Depth: 2}}}
	return d, i
}

func TestPackageViewsRanked(t *testing.T) {
	d, i := fixture()
	v := PackageViews(d, i)
	if len(v) == 0 {
		t.Fatal("no views")
	}
	if v[0].Package != "session" || v[0].Added != 1 {
		t.Fatalf("expected session first: %#v", v[0])
	}
	if v[0].Blast != 2 {
		t.Fatalf("expected transitive blast of 2, got %#v", v[0])
	}
	for _, x := range v {
		if x.Package == "handler" && x.Affected != 1 {
			t.Fatalf("handler affected: %d", x.Affected)
		}
	}
}

func TestClustersInducedSubgraph(t *testing.T) {
	d, i := fixture()
	cs := Clusters(d, i, 4, 14)
	if len(cs) == 0 || cs[0].Package != "session" {
		t.Fatalf("%#v", cs)
	}
	ids := map[string]bool{}
	for _, n := range cs[0].Diff.Nodes {
		ids[n.ID] = true
	}
	if !ids["v"] || !ids["h"] || !ids["r"] {
		t.Fatalf("cluster should include changed + reverse deps: %#v", ids)
	}
	if len(cs[0].Diff.Edges) != 2 {
		t.Fatalf("expected induced edges e1,e3, got %d", len(cs[0].Diff.Edges))
	}
}

func TestClustersNodeCap(t *testing.T) {
	d, i := fixture()
	cs := Clusters(d, i, 1, 2)
	if len(cs[0].Diff.Nodes) > 2 {
		t.Fatalf("cap violated: %d", len(cs[0].Diff.Nodes))
	}
}

func TestOverviewDiffPackages(t *testing.T) {
	d, i := fixture()
	od := OverviewDiff(d, i, 24)
	if len(od.Nodes) < 3 || len(od.Edges) == 0 {
		t.Fatalf("expected package nodes and aggregated edges: %#v", od)
	}
	seen := map[string]bool{}
	for _, n := range od.Nodes {
		if seen[n.ID] {
			t.Fatal("duplicate overview node")
		}
		seen[n.ID] = true
	}
}

func TestPackageNameGroupsPackageNodeWithSymbols(t *testing.T) {
	pkg := Node{Kind: "package", Name: "example.test/session-service/session", Location: Location{Path: "session"}}
	fn := Node{Kind: "function", QualifiedName: "example.test/session-service/session.Validate", Location: Location{Path: "session/validator.go"}}
	if PackageName(pkg) != PackageName(fn) {
		t.Fatalf("%q != %q", PackageName(pkg), PackageName(fn))
	}
	dep := Node{Kind: "external_dependency", Name: "github.com/redis/go-redis/v9"}
	if PackageName(dep) != "github.com/redis/go-redis/v9" {
		t.Fatalf("dependency: %q", PackageName(dep))
	}
}
