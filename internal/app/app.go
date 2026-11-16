package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/andrew-ships-stuff/impactr/internal/analysis"
	"github.com/andrew-ships-stuff/impactr/internal/config"
	gitx "github.com/andrew-ships-stuff/impactr/internal/git"
	"github.com/andrew-ships-stuff/impactr/internal/graph"
	"github.com/andrew-ships-stuff/impactr/internal/inference"
	"github.com/andrew-ships-stuff/impactr/internal/render"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Options struct {
	Base, Head                 string
	Mode                       gitx.ComparisonMode
	NoAI                       bool
	Provider, Model, OutputDir string
	Version                    string
}

func Run(ctx context.Context, start string, c config.Config, o Options) (render.Report, error) {
	repo, e := gitx.Open(ctx, start)
	if e != nil {
		return render.Report{}, e
	}
	var beforeRoot, afterRoot string
	var cb, ca func()
	var baseSHA, headSHA, comparison string
	switch o.Mode {
	case gitx.WorkingTree:
		baseSHA, e = rev(ctx, repo.Root, "HEAD")
		if e == nil {
			beforeRoot, cb, e = repo.Snapshot(ctx, "HEAD")
		}
		afterRoot, ca, e = repo.WorktreeSnapshot(ctx, c.Git.IncludeUntracked)
		headSHA = "WORKTREE"
		comparison = "working-tree"
	case gitx.Staged:
		baseSHA, e = rev(ctx, repo.Root, "HEAD")
		if e == nil {
			beforeRoot, cb, e = repo.Snapshot(ctx, "HEAD")
		}
		if e == nil {
			afterRoot, ca, e = repo.IndexSnapshot(ctx)
		}
		headSHA = "INDEX"
		comparison = "staged"
	default:
		sel, er := repo.Resolve(ctx, o.Base, o.Head, o.Mode)
		e = er
		if e == nil {
			baseSHA, headSHA, comparison = sel.BaseSHA, sel.HeadSHA, string(sel.Mode)
			beforeRoot, cb, e = repo.Snapshot(ctx, baseSHA)
		}
		if e == nil {
			afterRoot, ca, e = repo.Snapshot(ctx, headSHA)
		}
	}
	if e != nil {
		return render.Report{}, e
	}
	changes, e := repo.Changes(ctx, baseSHA, headSHA, o.Mode)
	if e != nil {
		return render.Report{}, e
	}
	if cb != nil {
		defer cb()
	}
	if ca != nil {
		defer ca()
	}
	eng := analysis.New()
	aopt := analysis.Options{Languages: c.Analysis.Languages, Exclude: c.Analysis.Exclude, IncludeTests: c.Analysis.IncludeTests, IncludeGenerated: c.Analysis.IncludeGenerated, Workers: c.Analysis.Concurrency}
	aopt.CacheDir = filepath.Join(repo.Root, ".impactr", "cache")
	var before, after graph.Graph
	var eb, ea error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); before, eb = eng.Analyze(ctx, beforeRoot, baseSHA, aopt) }()
	go func() { defer wg.Done(); after, ea = eng.Analyze(ctx, afterRoot, headSHA, aopt) }()
	wg.Wait()
	if eb != nil {
		return render.Report{}, eb
	}
	if ea != nil {
		return render.Report{}, ea
	}
	now := time.Now().UTC()
	prov := graph.Provenance{BaseSHA: baseSHA, HeadSHA: headSHA, Comparison: comparison, GeneratedAt: now, ImpactrVersion: o.Version, Analyzers: after.Provenance.Analyzers}
	before.Provenance = prov
	after.Provenance = prov
	d := graph.Compare(before, after)
	impact := graph.BlastRadius(d, c.Analysis.MaxDepth, c.Analysis.MaxNodes)
	req := inference.Request{Diff: d, Impact: impact, MaxTokens: c.Inference.MaxOutputTokens}
	ex := inference.Deterministic(req, "")
	if !o.NoAI && providerName(c, o) != "none" {
		p, er := makeProvider(c, o)
		if er == nil {
			ictx, cancel := context.WithTimeout(ctx, c.InferenceTimeout())
			got, ie := p.Explain(ictx, req)
			cancel()
			if ie == nil {
				ex = got
			} else {
				ex = inference.Deterministic(req, ie.Error())
			}
		} else {
			ex = inference.Deterministic(req, er.Error())
		}
	}
	report := render.Report{SchemaVersion: graph.SchemaVersion, Before: before, After: after, Diff: d, Impact: impact, Explanation: ex, Files: changes}
	out := o.OutputDir
	if out == "" {
		out = filepath.Join(repo.Root, ".impactr")
	}
	if e = WriteArtifacts(out, report, c.Render.MaxNodes); e != nil {
		return report, e
	}
	return report, nil
}
func providerName(c config.Config, o Options) string {
	if o.Provider != "" {
		return o.Provider
	}
	return c.Inference.Provider
}
func makeProvider(c config.Config, o Options) (inference.Provider, error) {
	name := providerName(c, o)
	model := o.Model
	if model == "" || model == "auto" {
		model = c.Inference.Model
	}
	switch name {
	case "embedded":
		m := inference.ModelManager{}
		p, e := m.Verify(inference.DefaultModel)
		if e != nil {
			return nil, fmt.Errorf("model unavailable; run `impactr model install`: %w", e)
		}
		return inference.Embedded{ModelPath: p, ModelName: inference.DefaultModel.Name, Threads: c.Inference.Threads, ContextLength: c.Inference.ContextLength, MaxMemoryMB: c.Inference.MaxMemoryMB}, nil
	case "ollama":
		ep := c.Inference.Endpoint
		if ep == "" {
			ep = "http://127.0.0.1:11434"
		}
		return inference.HTTPProvider{ProviderName: name, Endpoint: ep, Model: model}, nil
	case "llama-server", "openai-compatible":
		ep := c.Inference.Endpoint
		if ep == "" {
			ep = "http://127.0.0.1:8080"
		}
		return inference.HTTPProvider{ProviderName: name, Endpoint: ep, Model: model, APIKey: os.Getenv("IMPACTR_API_KEY")}, nil
	case "none":
		return nil, errors.New("inference disabled")
	default:
		return nil, fmt.Errorf("unknown provider %q", name)
	}
}
func WriteArtifacts(dir string, r render.Report, max int) error {
	if e := os.MkdirAll(dir, 0755); e != nil {
		return e
	}
	items := map[string]any{"analysis.json": r, "graph-before.json": r.Before, "graph-after.json": r.After, "graph-diff.json": r.Diff, "report.json": r}
	for name, v := range items {
		f, e := os.Create(filepath.Join(dir, name))
		if e != nil {
			return e
		}
		e = render.JSON(f, v)
		ce := f.Close()
		if e != nil {
			return e
		}
		if ce != nil {
			return ce
		}
	}
	svg, e := os.Create(filepath.Join(dir, "impact.svg"))
	if e != nil {
		return e
	}
	impactDiff := render.ImpactDiff(r.Diff, r.Impact)
	e = render.SVG(svg, impactDiff, max, false)
	svg.Close()
	if e != nil {
		return e
	}
	pngf, e := os.Create(filepath.Join(dir, "impact.png"))
	if e != nil {
		return e
	}
	e = render.PNG(pngf, impactDiff, max)
	pngf.Close()
	if e != nil {
		return e
	}
	mer, e := os.Create(filepath.Join(dir, "impact.mmd"))
	if e != nil {
		return e
	}
	e = render.Mermaid(mer, r.Diff, max)
	mer.Close()
	if e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(dir, "summary.md"), []byte(render.Markdown(r)), 0644); e != nil {
		return e
	}
	arch, e := os.Create(filepath.Join(dir, "architecture.svg"))
	if e != nil {
		return e
	}
	e = render.SVG(arch, r.Diff, max, false)
	_ = arch.Close()
	if e != nil {
		return e
	}
	archPNG, e := os.Create(filepath.Join(dir, "architecture.png"))
	if e != nil {
		return e
	}
	e = render.PNG(archPNG, r.Diff, max)
	_ = archPNG.Close()
	return e
}
func LoadReport(path string) (render.Report, error) {
	var r render.Report
	b, e := os.ReadFile(path)
	if e != nil {
		return r, e
	}
	e = json.Unmarshal(b, &r)
	return r, e
}
func rev(ctx context.Context, root, ref string) (string, error) {
	repo, e := gitx.Open(ctx, root)
	if e != nil {
		return "", e
	}
	s, e := repo.Resolve(ctx, ref, ref, gitx.Exact)
	return s.HeadSHA, e
}
