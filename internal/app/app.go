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
	"github.com/andrew-ships-stuff/impactr/internal/progress"
	"github.com/andrew-ships-stuff/impactr/internal/render"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Options struct {
	Base, Head                 string
	Mode                       gitx.ComparisonMode
	NoAI                       bool
	Provider, Model, OutputDir string
	Version                    string
	Progress                   *progress.Reporter
}

func Run(ctx context.Context, start string, c config.Config, o Options) (render.Report, error) {
	p := o.Progress
	repo, e := gitx.Open(ctx, start)
	if e != nil {
		return render.Report{}, e
	}
	p.Emit(progress.Event{Phase: "git", Detail: "repository " + filepath.Base(repo.Root), Done: true})
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
	p.Emit(progress.Event{Phase: "git", Detail: fmt.Sprintf("%s…%s (%d files changed)", short(baseSHA), short(headSHA), len(changes)), Done: true})
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
	go func() {
		defer wg.Done()
		t := time.Now()
		before, eb = eng.Analyze(ctx, beforeRoot, baseSHA, aopt)
		p.Emit(progress.Event{Phase: "analyze base", Detail: fmt.Sprintf("%d nodes · %d edges", len(before.Nodes), len(before.Edges)), Done: true, Err: eb, Elapsed: time.Since(t)})
	}()
	go func() {
		defer wg.Done()
		t := time.Now()
		after, ea = eng.Analyze(ctx, afterRoot, headSHA, aopt)
		p.Emit(progress.Event{Phase: "analyze head", Detail: fmt.Sprintf("%d nodes · %d edges", len(after.Nodes), len(after.Edges)), Done: true, Err: ea, Elapsed: time.Since(t)})
	}()
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
	p.Emit(progress.Event{Phase: "compare", Detail: fmt.Sprintf("+%d/−%d/~%d nodes · +%d/−%d edges · %d potentially affected", d.Summary.NodesAdded, d.Summary.NodesRemoved, d.Summary.NodesModified, d.Summary.EdgesAdded, d.Summary.EdgesRemoved, len(impact.Affected)), Done: true})
	if o.NoAI {
		p.Emit(progress.Event{Phase: "model", Detail: "skipped (--no-ai); deterministic facts only", Done: true})
	}
	views := graph.PackageViews(d, impact)
	clusters := graph.Clusters(d, impact, 4, 14)
	req := inference.Request{Diff: d, Impact: impact, MaxTokens: c.Inference.MaxOutputTokens}
	ex := inference.Deterministic(req, "")
	breq := inference.BriefRequest{Views: views, Clusters: clusters, MaxTokens: c.Inference.MaxOutputTokens}
	brief := inference.DeterministicBrief(breq, "")
	if !o.NoAI && providerName(c, o) != "none" {
		prov, er := makeProvider(c, o, p)
		if er == nil {
			if narrator, ok := prov.(inference.Narrator); ok {
				ictx, cancel := context.WithTimeout(ctx, 2*c.InferenceTimeout())
				got, gotBrief, ie := narrator.Narrate(ictx, req, breq)
				cancel()
				if ie == nil {
					ex, brief = got, gotBrief
				} else {
					ex = inference.Deterministic(req, ie.Error())
					brief = inference.DeterministicBrief(breq, ie.Error())
				}
			} else {
				bctx, bcancel := context.WithTimeout(ctx, c.InferenceTimeout())
				got, ie := prov.Explain(bctx, req)
				bcancel()
				if ie == nil {
					ex = got
				} else {
					ex = inference.Deterministic(req, ie.Error())
				}
				if b, ok := prov.(inference.Briefer); ok {
					briefCtx, briefCancel := context.WithTimeout(ctx, c.InferenceTimeout())
					gotBrief, briefErr := b.Brief(briefCtx, breq)
					briefCancel()
					if briefErr == nil {
						brief = gotBrief
					}
				}
			}
		} else {
			p.Emit(progress.Event{Phase: "model", Detail: "unavailable: " + er.Error() + "; deterministic fallback", Done: true})
			ex = inference.Deterministic(req, er.Error())
			brief = inference.DeterministicBrief(breq, er.Error())
		}
	}
	report := render.Report{SchemaVersion: graph.SchemaVersion, Before: before, After: after, Diff: d, Impact: impact, Explanation: ex, Files: changes, Brief: &brief, Rollup: views}
	report.Images = artifactImages(report, clusters)
	out := o.OutputDir
	if out == "" {
		out = filepath.Join(repo.Root, ".impactr")
	}
	if e = WriteArtifacts(out, report, c.Render.MaxNodes); e != nil {
		return report, e
	}
	n := 0
	if entries, le := os.ReadDir(out); le == nil {
		n = len(entries)
	}
	p.Emit(progress.Event{Phase: "artifacts", Detail: fmt.Sprintf("%d files → %s", n, out), Done: true})
	return report, nil
}
func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
func providerName(c config.Config, o Options) string {
	if o.Provider != "" {
		return o.Provider
	}
	return c.Inference.Provider
}
func makeProvider(c config.Config, o Options, rep *progress.Reporter) (inference.Provider, error) {
	name := providerName(c, o)
	model := o.Model
	if model == "" || model == "auto" {
		model = c.Inference.Model
	}
	emit := func(ev progress.Event) {
		if rep != nil {
			rep.Emit(ev)
		}
	}
	switch name {
	case "embedded":
		m := inference.ModelManager{}
		p, e := m.Verify(inference.DefaultModel)
		if e != nil {
			return nil, fmt.Errorf("model unavailable; run `impactr model install`: %w", e)
		}
		return inference.Embedded{ModelPath: p, ModelName: inference.DefaultModel.Name, Threads: c.Inference.Threads, ContextLength: c.Inference.ContextLength, MaxMemoryMB: c.Inference.MaxMemoryMB, Report: emit}, nil
	case "ollama":
		ep := c.Inference.Endpoint
		if ep == "" {
			ep = "http://127.0.0.1:11434"
		}
		return inference.HTTPProvider{ProviderName: name, Endpoint: ep, Model: model, Report: emit}, nil
	case "llama-server", "openai-compatible":
		ep := c.Inference.Endpoint
		if ep == "" {
			ep = "http://127.0.0.1:8080"
		}
		return inference.HTTPProvider{ProviderName: name, Endpoint: ep, Model: model, APIKey: os.Getenv("IMPACTR_API_KEY"), Report: emit}, nil
	case "none":
		return nil, errors.New("inference disabled")
	default:
		return nil, fmt.Errorf("unknown provider %q", name)
	}
}
func artifactImages(r render.Report, clusters []graph.Cluster) []render.Image {
	imgs := []render.Image{{Path: "impact.png", Caption: "Package impact overview"}}
	for i, cl := range clusters {
		caption := cl.Package + " data flow"
		if r.Brief != nil {
			for _, c := range r.Brief.Clusters {
				if c.Package == cl.Package && strings.TrimSpace(c.Title) != "" {
					caption = cl.Package + " — " + c.Title
				}
			}
		}
		imgs = append(imgs, render.Image{Path: fmt.Sprintf("impact-%d.png", i+1), Caption: caption})
	}
	return imgs
}
func WriteArtifacts(dir string, r render.Report, max int) error {
	if e := os.MkdirAll(dir, 0755); e != nil {
		return e
	}
	items := map[string]any{"analysis.json": r, "graph-before.json": r.Before, "graph-after.json": r.After, "graph-diff.json": r.Diff, "report.json": r}
	for name, v := range items {
		if e := writeArtifact(filepath.Join(dir, name), func(w io.Writer) error { return render.JSON(w, v) }); e != nil {
			return e
		}
	}
	overview := graph.OverviewDiff(r.Diff, r.Impact, 24)
	if e := writeArtifact(filepath.Join(dir, "impact.svg"), func(w io.Writer) error { return render.SVG(w, overview, max, false) }); e != nil {
		return e
	}
	if e := writeArtifact(filepath.Join(dir, "impact.png"), func(w io.Writer) error { return render.PNG(w, overview, max) }); e != nil {
		return e
	}
	for i, cl := range graph.Clusters(r.Diff, r.Impact, 4, 14) {
		if e := writeArtifact(filepath.Join(dir, fmt.Sprintf("impact-%d.svg", i+1)), func(w io.Writer) error {
			return render.SVGTitled(w, cl.Diff, max, false, cl.Package+" data flow")
		}); e != nil {
			return e
		}
		if e := writeArtifact(filepath.Join(dir, fmt.Sprintf("impact-%d.png", i+1)), func(w io.Writer) error { return render.PNG(w, cl.Diff, max) }); e != nil {
			return e
		}
	}
	if e := writeArtifact(filepath.Join(dir, "impact.mmd"), func(w io.Writer) error { return render.Mermaid(w, r.Diff, max) }); e != nil {
		return e
	}
	if e := os.WriteFile(filepath.Join(dir, "summary.md"), []byte(render.Markdown(r)), 0644); e != nil {
		return e
	}
	if e := writeArtifact(filepath.Join(dir, "architecture.svg"), func(w io.Writer) error { return render.SVG(w, r.Diff, max, false) }); e != nil {
		return e
	}
	return writeArtifact(filepath.Join(dir, "architecture.png"), func(w io.Writer) error { return render.PNG(w, r.Diff, max) })
}

func writeArtifact(path string, write func(io.Writer) error) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err = write(f); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
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
