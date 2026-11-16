package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/andrew-ships-stuff/impactr/internal/cache"
	"github.com/andrew-ships-stuff/impactr/internal/graph"
)

type Options struct {
	Languages                      []string
	Exclude                        []string
	IncludeTests, IncludeGenerated bool
	Workers                        int
	CacheDir                       string
}
type Analyzer interface {
	Name() string
	Version() string
	Extensions() []string
	Analyze(context.Context, string, []string) ([]graph.Node, []graph.Edge, []string, error)
}
type Engine struct{ analyzers []Analyzer }

func New() *Engine {
	return &Engine{analyzers: []Analyzer{GoAnalyzer{}, ScriptAnalyzer{language: "typescript", extensions: []string{".ts", ".tsx", ".js", ".jsx"}}, ScriptAnalyzer{language: "python", extensions: []string{".py"}}}}
}

func (e *Engine) Analyze(ctx context.Context, root, revision string, opt Options) (graph.Graph, error) {
	if opt.Workers <= 0 {
		opt.Workers = runtime.GOMAXPROCS(0)
	}
	if opt.Workers > 32 {
		opt.Workers = 32
	}
	enabled := map[string]bool{}
	for _, l := range opt.Languages {
		enabled[l] = true
	}
	if len(enabled) == 0 {
		enabled["go"] = true
		enabled["typescript"] = true
		enabled["python"] = true
	}
	key, err := cacheKey(root, opt, e.analyzers)
	if err != nil {
		return graph.Graph{}, err
	}
	store := cache.Store{Dir: opt.CacheDir}
	if got, ok, err := store.Load(key); err != nil {
		return graph.Graph{}, err
	} else if ok {
		got.Revision = revision
		return got, nil
	}
	byExt := map[string]Analyzer{}
	for _, a := range e.analyzers {
		if enabled[a.Name()] {
			for _, x := range a.Extensions() {
				byExt[x] = a
			}
		}
	}
	filesBy := map[string][]string{}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == ".git" || rel == ".impactr" || rel == "vendor" || rel == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		a := byExt[filepath.Ext(path)]
		if a == nil || excluded(rel, opt.Exclude) || (!opt.IncludeTests && isTest(rel)) {
			return nil
		}
		if !opt.IncludeGenerated {
			b, _ := os.ReadFile(path)
			if strings.Contains(string(b), "Code generated") || strings.Contains(string(b), "@generated") {
				return nil
			}
		}
		filesBy[a.Name()] = append(filesBy[a.Name()], path)
		return nil
	})
	if err != nil {
		return graph.Graph{}, err
	}
	g := graph.Graph{Revision: revision, Provenance: graph.Provenance{Analyzers: map[string]string{}}}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, opt.Workers)
	errCh := make(chan error, len(filesBy))
	for _, a := range e.analyzers {
		files := filesBy[a.Name()]
		if len(files) == 0 {
			continue
		}
		sort.Strings(files)
		wg.Add(1)
		go func(a Analyzer, files []string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			}
			defer func() { <-sem }()
			n, ed, w, er := a.Analyze(ctx, root, files)
			if er != nil {
				errCh <- fmt.Errorf("%s analyzer: %w", a.Name(), er)
				return
			}
			mu.Lock()
			g.Nodes = append(g.Nodes, n...)
			g.Edges = append(g.Edges, ed...)
			g.Warnings = append(g.Warnings, w...)
			g.Provenance.Analyzers[a.Name()] = a.Version()
			mu.Unlock()
		}(a, files)
	}
	wg.Wait()
	close(errCh)
	for er := range errCh {
		if er != nil {
			return graph.Graph{}, er
		}
	}
	g.Normalize()
	if err := store.Save(key, g); err != nil {
		return graph.Graph{}, err
	}
	return g, nil
}

func cacheKey(root string, opt Options, analyzers []Analyzer) (string, error) {
	h := sha256.New()
	fmt.Fprintf(h, "impactr-analysis-v1\x00%v\x00%v\x00%t\x00%t", opt.Languages, opt.Exclude, opt.IncludeTests, opt.IncludeGenerated)
	for _, a := range analyzers {
		fmt.Fprintf(h, "\x00%s:%s", a.Name(), a.Version())
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == ".git" || rel == ".impactr" || rel == "vendor" || rel == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		supported := false
		for _, a := range analyzers {
			for _, x := range a.Extensions() {
				if ext == x {
					supported = true
				}
			}
		}
		if !supported || excluded(rel, opt.Exclude) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fmt.Fprintf(h, "\x00%s\x00", rel)
		_, _ = h.Write(b)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func excluded(path string, patterns []string) bool {
	for _, p := range patterns {
		p = strings.TrimPrefix(filepath.ToSlash(p), "./")
		if ok, _ := filepath.Match(p, path); ok {
			return true
		}
		if strings.HasSuffix(p, "/**") && strings.HasPrefix(path, strings.TrimSuffix(p, "**")) {
			return true
		}
		if strings.HasPrefix(p, "**/") {
			if ok, _ := filepath.Match(strings.TrimPrefix(p, "**/"), filepath.Base(path)); ok {
				return true
			}
		}
	}
	return false
}
func isTest(path string) bool {
	return strings.HasSuffix(path, "_test.go") || strings.Contains(path, ".test.") || strings.Contains(path, "/test/") || strings.HasPrefix(filepath.Base(path), "test_")
}
