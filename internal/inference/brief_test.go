package inference

import (
	"strings"
	"testing"

	"github.com/andrew-ships-stuff/impactr/internal/graph"
)

func briefReq() BriefRequest {
	return BriefRequest{
		Views: []graph.PackageView{{Package: "session", Added: 2, Affected: 5}, {Package: "handler", Modified: 1, Affected: 3}},
		Clusters: []graph.Cluster{
			{Package: "session", Changed: []string{"a"}},
			{Package: "handler", Changed: []string{"b"}},
		},
	}
}

func TestParseBriefValid(t *testing.T) {
	raw := `{"headline":"session validation added","clusters":[{"package":"session","title":"validation flow"},{"package":"ghost","title":"invented"},{"package":"session","title":"dupe"},{"package":"handler","title":""}]}`
	b, e := ParseBrief(raw, briefReq())
	if e != nil {
		t.Fatal(e)
	}
	if b.Headline != "session validation added" {
		t.Fatalf("headline: %s", b.Headline)
	}
	if len(b.Clusters) != 1 || b.Clusters[0].Package != "session" || b.Clusters[0].Title != "validation flow" {
		t.Fatalf("unknown/empty/duplicate packages must be dropped: %#v", b.Clusters)
	}
}

func TestParseBriefRejectsEmptyHeadline(t *testing.T) {
	if _, e := ParseBrief(`{"headline":"","clusters":[]}`, briefReq()); e == nil {
		t.Fatal("expected empty headline rejection")
	}
}

func TestBriefPromptNamesOnlyKnownPackages(t *testing.T) {
	p := BriefPrompt(briefReq())
	if !strings.Contains(p, "session") || !strings.Contains(p, "handler") || strings.Contains(p, "ghost") {
		t.Fatalf("prompt must list exactly the known packages: %s", p)
	}
}

func TestDeterministicBrief(t *testing.T) {
	b := DeterministicBrief(briefReq(), "")
	if !b.Fallback || b.Provider != "deterministic" {
		t.Fatalf("%#v", b)
	}
	if !strings.Contains(b.Headline, "3 symbols changed across 2 local packages") || !strings.Contains(b.Headline, "Review session first") {
		t.Fatalf("headline: %s", b.Headline)
	}
	if len(b.Clusters) != 2 || b.Clusters[0].Title == "" {
		t.Fatalf("clusters: %#v", b.Clusters)
	}
}

func TestValidateBriefKeepsValidTitles(t *testing.T) {
	b, e := ValidateBrief(Brief{Headline: "handler serves requests", Clusters: []ClusterBrief{{Package: "handler", Title: "serve path"}}}, briefReq())
	if e != nil || len(b.Clusters) != 1 {
		t.Fatalf("%#v %v", b, e)
	}
}

func TestValidateBriefRejectsUngroundedOrUnreadableHeadline(t *testing.T) {
	for _, headline := range []string{
		"unrelated system changes",
		"内部渲染, session, handler, added: 3, removed: 2, modified: 4, affected: 8, blast: 9",
	} {
		if _, err := ValidateBrief(Brief{Headline: headline}, briefReq()); err == nil {
			t.Fatalf("expected rejection for %q", headline)
		}
	}
}
