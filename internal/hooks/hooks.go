package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const marker = "# impactr-managed-v1"

func Install(gitDir, kind string, blocking bool) error {
	if kind != "pre-commit" && kind != "pre-push" {
		return fmt.Errorf("unsupported hook type %q", kind)
	}
	path := filepath.Join(gitDir, "hooks", kind)
	if e := os.MkdirAll(filepath.Dir(path), 0755); e != nil {
		return e
	}
	orig := path + ".impactr-original"
	if b, e := os.ReadFile(path); e == nil && !strings.Contains(string(b), marker) {
		if _, e = os.Stat(orig); e == nil {
			return fmt.Errorf("refusing to replace hook: backup already exists at %s", orig)
		}
		if e = os.Rename(path, orig); e != nil {
			return e
		}
	}
	impact := "impactr analyze --no-ai --format terminal --hook-mode " + kind
	if !blocking {
		impact += " || echo 'impactr: analysis warning (non-blocking)' >&2"
	}
	script := "#!/bin/sh\n" + marker + "\nstatus=0\nif [ -x \"" + orig + "\" ]; then \"" + orig + "\" \"$@\" || status=$?; fi\nif [ $status -ne 0 ]; then exit $status; fi\n" + impact + "\nexit $?\n"
	return os.WriteFile(path, []byte(script), 0755)
}
func Uninstall(gitDir, kind string) error {
	path := filepath.Join(gitDir, "hooks", kind)
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	if !strings.Contains(string(b), marker) {
		return fmt.Errorf("%s is not managed by Impactr", path)
	}
	orig := path + ".impactr-original"
	if _, e = os.Stat(orig); e == nil {
		if e = os.Remove(path); e != nil {
			return e
		}
		return os.Rename(orig, path)
	}
	return os.Remove(path)
}
