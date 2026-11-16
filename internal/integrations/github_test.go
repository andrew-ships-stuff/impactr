package integrations

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type fake struct {
	n    string
	a    []string
	body string
}

func (f *fake) Run(_ context.Context, n string, a ...string) error {
	f.n = n
	f.a = a
	if i := indexOf(a, "--body-file"); i >= 0 && i+1 < len(a) {
		if b, e := os.ReadFile(a[i+1]); e == nil {
			f.body = string(b)
		}
	}
	return nil
}

type fakeQuery struct {
	fake
	out string
}

func (f *fakeQuery) Output(context.Context, string, ...string) ([]byte, error) {
	return []byte(f.out), nil
}

func writeSummary(t *testing.T, stats string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "summary.md")
	body := "<!-- impactr-comment:v1 -->\n<!-- impactr-stats:v1 " + stats + " -->\n## Impactr architecture impact\n"
	if e := os.WriteFile(p, []byte(body), 0600); e != nil {
		t.Fatal(e)
	}
	return p
}

func TestGitHubIdempotentArgs(t *testing.T) {
	f := &fake{}
	if e := GitHubComment(context.Background(), f, 12, "o/r", "summary.md", "impact.png"); e != nil {
		t.Fatal(e)
	}
	want := []string{"pr", "comment", "12", "--body-file", "summary.md", "--edit-last", "--create-if-none", "--repo", "o/r", "--attach", "impact.png#Impactr architecture impact diagram"}
	if f.n != "gh" || !reflect.DeepEqual(f.a, want) {
		t.Fatalf("%s %#v", f.n, f.a)
	}
}

func TestGitHubCommentEditsWhenSimilar(t *testing.T) {
	f := &fakeQuery{out: "8 7 2\n"}
	body := writeSummary(t, "nodes=8 edges=7 affected=2 head=abc1234")
	if e := GitHubComment(context.Background(), f, 12, "o/r", body, "impact.png"); e != nil {
		t.Fatal(e)
	}
	if !hasFlag(f.a, "--edit-last") || !hasFlag(f.a, "--create-if-none") {
		t.Fatalf("expected in-place edit, got %#v", f.a)
	}
	if f.a[4] != body {
		t.Fatalf("expected original body file, got %s", f.a[4])
	}
}

func TestGitHubCommentNewCommentWhenSubstantial(t *testing.T) {
	f := &fakeQuery{out: "1 0 0\n"}
	body := writeSummary(t, "nodes=8 edges=7 affected=2 head=abc1234")
	if e := GitHubComment(context.Background(), f, 12, "o/r", body, "impact.png"); e != nil {
		t.Fatal(e)
	}
	if hasFlag(f.a, "--edit-last") || hasFlag(f.a, "--create-if-none") {
		t.Fatalf("expected fresh comment, got %#v", f.a)
	}
	if f.a[4] == body {
		t.Fatal("expected supersede note written to a temp body file")
	}
	if !strings.HasPrefix(f.body, "> Supersedes") || !strings.Contains(f.body, "impactr-comment:v1") {
		t.Fatalf("missing supersede note: %s", f.body)
	}
	if _, e := os.Stat(f.a[4]); !os.IsNotExist(e) {
		t.Fatal("temp body file should be cleaned up after publishing")
	}
}

func TestGitHubCommentShrinkIsSubstantial(t *testing.T) {
	f := &fakeQuery{out: "40 30 20\n"}
	body := writeSummary(t, "nodes=3 edges=2 affected=1 head=abc1234")
	if e := GitHubComment(context.Background(), f, 12, "o/r", body, ""); e != nil {
		t.Fatal(e)
	}
	if hasFlag(f.a, "--edit-last") {
		t.Fatalf("sudden shrink should post a new comment, got %#v", f.a)
	}
}

func TestParseStats(t *testing.T) {
	s, ok := ParseStats("<!-- impactr-stats:v1 nodes=8 edges=7 affected=2 head=abc -->")
	if !ok || s != (Stats{Nodes: 8, Edges: 7, Affected: 2}) {
		t.Fatalf("%#v %v", s, ok)
	}
	if _, ok = ParseStats("no stats here"); ok {
		t.Fatal("expected no stats")
	}
}

func hasFlag(a []string, flag string) bool {
	for _, v := range a {
		if v == flag {
			return true
		}
	}
	return false
}

func indexOf(a []string, want string) int {
	for i, v := range a {
		if v == want {
			return i
		}
	}
	return -1
}
