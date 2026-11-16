package artifact

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRejectsSymlink(t *testing.T) {
	d := t.TempDir()
	target := filepath.Join(d, "target")
	_ = os.WriteFile(target, []byte("x"), 0600)
	if err := os.Symlink(target, filepath.Join(d, "link")); err != nil {
		t.Skip(err)
	}
	if Validate(d) == nil {
		t.Fatal("expected rejection")
	}
}
