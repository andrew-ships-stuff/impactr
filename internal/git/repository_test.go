package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestResolveAndSnapshot(t *testing.T) {
	d := t.TempDir()
	cmd := exec.Command("git", "init", "-b", "main")
	cmd.Dir = d
	if out, e := cmd.CombinedOutput(); e != nil {
		t.Fatal(e, string(out))
	}
	_ = os.WriteFile(filepath.Join(d, "go.mod"), []byte("module example.test/x\n"), 0644)
	for _, a := range [][]string{{"add", "."}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", "init"}} {
		c := exec.Command("git", a...)
		c.Dir = d
		if out, e := c.CombinedOutput(); e != nil {
			t.Fatal(e, string(out))
		}
	}
	r, e := Open(context.Background(), d)
	if e != nil {
		t.Fatal(e)
	}
	s, e := r.Resolve(context.Background(), "main", "HEAD", Exact)
	if e != nil || s.BaseSHA == "" {
		t.Fatal(e)
	}
	snap, done, e := r.Snapshot(context.Background(), "HEAD")
	if e != nil {
		t.Fatal(e)
	}
	defer done()
	if _, e = os.Stat(filepath.Join(snap, "go.mod")); e != nil {
		t.Fatal(e)
	}
}
