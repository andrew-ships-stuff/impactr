// Package git provides safe, read-only Git revision and snapshot operations.
package git

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type ComparisonMode string

const (
	MergeBase   ComparisonMode = "merge-base"
	Exact       ComparisonMode = "exact"
	Staged      ComparisonMode = "staged"
	WorkingTree ComparisonMode = "working-tree"
)

type Selection struct {
	Root, BaseRef, HeadRef, BaseSHA, HeadSHA string
	Mode                                     ComparisonMode
}
type FileChange struct {
	Status  string `json:"status"`
	Path    string `json:"path"`
	OldPath string `json:"old_path,omitempty"`
}
type Repository struct{ Root string }

func Open(ctx context.Context, start string) (Repository, error) {
	out, err := run(ctx, start, "rev-parse", "--show-toplevel")
	if err != nil {
		return Repository{}, errors.New("not inside a Git repository")
	}
	return Repository{Root: strings.TrimSpace(out)}, nil
}
func (r Repository) Resolve(ctx context.Context, base, head string, mode ComparisonMode) (Selection, error) {
	if head == "" {
		head = "HEAD"
	}
	if mode == "" {
		mode = MergeBase
	}
	if base == "" {
		var err error
		base, err = r.defaultBranch(ctx)
		if err != nil {
			return Selection{}, err
		}
	}
	bs, err := r.sha(ctx, base)
	if err != nil {
		return Selection{}, fmt.Errorf("base ref %q is unavailable (fetch sufficient history or pass --base): %w", base, err)
	}
	hs, err := r.sha(ctx, head)
	if err != nil {
		return Selection{}, fmt.Errorf("head ref %q: %w", head, err)
	}
	if mode == MergeBase {
		mb, e := run(ctx, r.Root, "merge-base", bs, hs)
		if e != nil {
			return Selection{}, fmt.Errorf("no merge base between %s and %s; shallow clone may need fetch-depth: 0", base, head)
		}
		bs = strings.TrimSpace(mb)
	}
	return Selection{Root: r.Root, BaseRef: base, HeadRef: head, BaseSHA: bs, HeadSHA: hs, Mode: mode}, nil
}
func (r Repository) defaultBranch(ctx context.Context) (string, error) {
	candidates := []string{}
	if s, e := run(ctx, r.Root, "symbolic-ref", "--quiet", "refs/remotes/origin/HEAD"); e == nil {
		candidates = append(candidates, strings.TrimPrefix(strings.TrimSpace(s), "refs/remotes/"))
	}
	candidates = append(candidates, "origin/main", "origin/master", "main", "master")
	for _, c := range candidates {
		if _, e := r.sha(ctx, c); e == nil {
			return c, nil
		}
	}
	return "", errors.New("cannot discover a default branch; pass --base explicitly")
}
func (r Repository) sha(ctx context.Context, ref string) (string, error) {
	s, e := run(ctx, r.Root, "rev-parse", "--verify", ref+"^{commit}")
	return strings.TrimSpace(s), e
}
func (r Repository) Snapshot(ctx context.Context, ref string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "impactr-snapshot-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	cmd := exec.CommandContext(ctx, "git", "archive", "--format=tar", ref)
	cmd.Dir = r.Root
	raw, err := cmd.Output()
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("archive %s: %w", ref, err)
	}
	tr := tar.NewReader(bytes.NewReader(raw))
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			cleanup()
			return "", nil, e
		}
		target := filepath.Join(dir, filepath.Clean(h.Name))
		if !strings.HasPrefix(target, dir+string(os.PathSeparator)) {
			cleanup()
			return "", nil, errors.New("unsafe path in Git archive")
		}
		switch h.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(target, 0755)
		case tar.TypeReg:
			if err = os.MkdirAll(filepath.Dir(target), 0755); err == nil {
				var f *os.File
				f, err = os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(h.Mode)&0755)
				if err == nil {
					_, err = io.Copy(f, tr)
					_ = f.Close()
				}
			}
		}
		if err != nil {
			cleanup()
			return "", nil, err
		}
	}
	return dir, cleanup, nil
}
func (r Repository) IndexSnapshot(ctx context.Context) (string, func(), error) {
	dir, err := os.MkdirTemp("", "impactr-index-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	prefix := dir + string(os.PathSeparator)
	cmd := exec.CommandContext(ctx, "git", "checkout-index", "--all", "--prefix="+prefix)
	cmd.Dir = r.Root
	if out, e := cmd.CombinedOutput(); e != nil {
		cleanup()
		return "", nil, fmt.Errorf("read index: %w: %s", e, out)
	}
	return dir, cleanup, nil
}

func (r Repository) WorktreeSnapshot(ctx context.Context, includeUntracked bool) (string, func(), error) {
	dir, err := os.MkdirTemp("", "impactr-worktree-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	args := []string{"ls-files", "-z"}
	if includeUntracked {
		args = []string{"ls-files", "-z", "--cached", "--others", "--exclude-standard"}
	}
	out, err := run(ctx, r.Root, args...)
	if err != nil {
		cleanup()
		return "", nil, err
	}
	for _, rel := range strings.Split(out, "\x00") {
		if rel == "" {
			continue
		}
		src := filepath.Join(r.Root, filepath.FromSlash(rel))
		info, er := os.Lstat(src)
		if errors.Is(er, os.ErrNotExist) {
			continue
		}
		if er != nil {
			cleanup()
			return "", nil, er
		}
		if !info.Mode().IsRegular() {
			continue
		}
		dst := filepath.Join(dir, filepath.FromSlash(rel))
		if er = os.MkdirAll(filepath.Dir(dst), 0755); er != nil {
			cleanup()
			return "", nil, er
		}
		in, er := os.Open(src)
		if er != nil {
			cleanup()
			return "", nil, er
		}
		of, er := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
		if er == nil {
			_, er = io.Copy(of, in)
			_ = of.Close()
		}
		_ = in.Close()
		if er != nil {
			cleanup()
			return "", nil, er
		}
	}
	return dir, cleanup, nil
}

func (r Repository) Changes(ctx context.Context, base, head string, mode ComparisonMode) ([]FileChange, error) {
	args := []string{"diff", "--name-status", "-z", "-M"}
	switch mode {
	case Staged:
		args = append(args, "--cached")
	case WorkingTree:
	default:
		args = append(args, base, head)
	}
	out, err := run(ctx, r.Root, args...)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(out, "\x00")
	var changes []FileChange
	for i := 0; i < len(parts) && parts[i] != ""; {
		status := parts[i]
		i++
		if i >= len(parts) {
			break
		}
		if strings.HasPrefix(status, "R") {
			if i+1 >= len(parts) {
				break
			}
			changes = append(changes, FileChange{Status: "renamed", OldPath: parts[i], Path: parts[i+1]})
			i += 2
			continue
		}
		name := map[byte]string{'A': "added", 'D': "removed", 'M': "modified"}[status[0]]
		if name == "" {
			name = strings.ToLower(status)
		}
		changes = append(changes, FileChange{Status: name, Path: parts[i]})
		i++
	}
	return changes, nil
}
func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
