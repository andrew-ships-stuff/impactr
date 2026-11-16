package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChainsAndRestores(t *testing.T) {
	d := t.TempDir()
	_ = os.MkdirAll(filepath.Join(d, "hooks"), 0755)
	p := filepath.Join(d, "hooks", "pre-push")
	old := []byte("#!/bin/sh\necho old\n")
	_ = os.WriteFile(p, old, 0755)
	if e := Install(d, "pre-push", false); e != nil {
		t.Fatal(e)
	}
	if e := Uninstall(d, "pre-push"); e != nil {
		t.Fatal(e)
	}
	got, _ := os.ReadFile(p)
	if string(got) != string(old) {
		t.Fatal("hook not restored")
	}
}
