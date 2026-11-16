package integrations

import (
	"context"
	"reflect"
	"testing"
)

type fake struct {
	n string
	a []string
}

func (f *fake) Run(_ context.Context, n string, a ...string) error { f.n = n; f.a = a; return nil }
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
