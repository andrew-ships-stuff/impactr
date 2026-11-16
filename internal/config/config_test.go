package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnknownFieldRejected(t *testing.T) {
	d := t.TempDir()
	if e := os.WriteFile(filepath.Join(d, ".impactr.yaml"), []byte("version: 1\nunknown: true\n"), 0644); e != nil {
		t.Fatal(e)
	}
	if _, e := Load(d); e == nil {
		t.Fatal("expected schema error")
	}
}
