package main

import (
	"context"
	"fmt"
	"github.com/andrew-ships-stuff/impactr/internal/app"
	"github.com/andrew-ships-stuff/impactr/internal/artifact"
	"github.com/andrew-ships-stuff/impactr/internal/config"
	gitx "github.com/andrew-ships-stuff/impactr/internal/git"
	"github.com/andrew-ships-stuff/impactr/internal/hooks"
	"github.com/andrew-ships-stuff/impactr/internal/inference"
	"github.com/andrew-ships-stuff/impactr/internal/integrations"
	"github.com/andrew-ships-stuff/impactr/internal/render"
	"github.com/spf13/cobra"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var version = "dev"

type flags struct {
	base, head, format, provider, model, output, view, hookMode string
	working, staged, noAI                                       bool
}

func main() {
	root := newRoot()
	if e := root.Execute(); e != nil {
		fmt.Fprintln(os.Stderr, "impactr:", e)
		os.Exit(1)
	}
}
func newRoot() *cobra.Command {
	var f flags
	r := &cobra.Command{Use: "impactr", Short: "See the impact before you merge.", SilenceUsage: true}
	r.Version = version
	an := &cobra.Command{Use: "analyze", Short: "Analyze architecture changes", RunE: func(cmd *cobra.Command, args []string) error { return analyze(cmd.Context(), f) }}
	an.Flags().StringVar(&f.base, "base", "", "base branch or commit")
	an.Flags().StringVar(&f.head, "head", "HEAD", "head branch or commit")
	an.Flags().BoolVar(&f.working, "working-tree", false, "compare HEAD with the working tree")
	an.Flags().BoolVar(&f.staged, "staged", false, "compare HEAD with the index")
	an.Flags().BoolVar(&f.noAI, "no-ai", false, "disable inference")
	an.Flags().StringVar(&f.provider, "provider", "", "embedded, none, ollama, llama-server, or openai-compatible")
	an.Flags().StringVar(&f.model, "model", "", "model name")
	an.Flags().StringVar(&f.format, "format", "terminal", "terminal or json")
	an.Flags().StringVar(&f.output, "output", "", "artifact directory (default .impactr)")
	an.Flags().StringVar(&f.view, "view", "diff", "architecture, diff, or impact")
	an.Flags().StringVar(&f.hookMode, "hook-mode", "", "internal hook mode")
	r.AddCommand(an, renderCmd(&f), modelCmd(), hookCmd(), githubCmd(&f), gitlabCmd(&f), validateCmd())
	return r
}
func analyze(ctx context.Context, f flags) error {
	if f.working && f.staged {
		return fmt.Errorf("--working-tree and --staged are mutually exclusive")
	}
	cwd, _ := os.Getwd()
	c, e := config.Load(cwd)
	if e != nil {
		return e
	}
	mode := gitx.ComparisonMode(c.Git.Comparison)
	if f.working {
		mode = gitx.WorkingTree
	} else if f.staged {
		mode = gitx.Staged
	}
	base := f.base
	if base == "" && c.Git.Base != "auto" {
		base = c.Git.Base
	}
	if f.noAI {
		f.provider = "none"
	}
	if f.hookMode == "pre-commit" {
		mode = gitx.Staged
		f.noAI = true
	}
	opts := app.Options{Base: base, Head: f.head, Mode: mode, NoAI: f.noAI, Provider: f.provider, Model: f.model, OutputDir: f.output, Version: version}
	if f.format == "terminal" {
		opts.Progress = newTerminalReporter()
	}
	rep, e := app.Run(ctx, cwd, c, opts)
	if e != nil {
		return e
	}
	if f.format == "json" {
		return render.JSON(os.Stdout, rep)
	}
	if f.view == "impact" {
		rep.Diff = render.ImpactDiff(rep.Diff, rep.Impact)
	} else if f.view != "diff" && f.view != "architecture" {
		return fmt.Errorf("unsupported view %q", f.view)
	}
	return render.Terminal(os.Stdout, rep)
}
func renderCmd(f *flags) *cobra.Command {
	var input, format, theme string
	c := &cobra.Command{Use: "render", Short: "Render an existing analysis", RunE: func(cmd *cobra.Command, args []string) error {
		r, e := app.LoadReport(input)
		if e != nil {
			return e
		}
		dark := theme == "dark"
		switch format {
		case "svg":
			return render.SVG(os.Stdout, r.Diff, 100, dark)
		case "png":
			return render.PNG(os.Stdout, r.Diff, 100)
		case "mermaid":
			return render.Mermaid(os.Stdout, r.Diff, 100)
		case "markdown":
			fmt.Print(render.Markdown(r))
			return nil
		default:
			return fmt.Errorf("unsupported format %q", format)
		}
	}}
	c.Flags().StringVar(&input, "input", ".impactr/analysis.json", "analysis JSON")
	c.Flags().StringVar(&format, "format", "svg", "svg, png, mermaid, or markdown")
	c.Flags().StringVar(&theme, "theme", "light", "light or dark")
	return c
}
func modelCmd() *cobra.Command {
	m := &cobra.Command{Use: "model", Short: "Manage embedded models"}
	install := &cobra.Command{Use: "install", Short: "Download and verify the default GGUF model", RunE: func(cmd *cobra.Command, args []string) error {
		mod := inference.DefaultModel
		fmt.Printf("Model: %s (%d MiB)\nLicense: %s — %s\nSHA-256: %s\n", mod.Name, mod.Size>>20, mod.License, mod.LicenseURL, mod.SHA256)
		last := time.Now()
		p, e := ((inference.ModelManager{}).Install(cmd.Context(), mod, func(n, total int64) {
			if time.Since(last) > time.Second || n == total {
				fmt.Fprintf(os.Stderr, "\rDownloading: %d/%d MiB", n>>20, total>>20)
				last = time.Now()
			}
		}))
		fmt.Fprintln(os.Stderr)
		if e == nil {
			fmt.Println("Installed:", p)
		}
		return e
	}}
	m.AddCommand(install)
	return m
}
func hookCmd() *cobra.Command {
	var kind string
	var blocking bool
	h := &cobra.Command{Use: "hook", Short: "Manage local Git hooks"}
	install := &cobra.Command{Use: "install", RunE: func(cmd *cobra.Command, args []string) error {
		d, e := gitDir(cmd.Context())
		if e != nil {
			return e
		}
		if kind == "" {
			kind = "pre-push"
		}
		return hooks.Install(d, kind, blocking)
	}}
	uninstall := &cobra.Command{Use: "uninstall", RunE: func(cmd *cobra.Command, args []string) error {
		d, e := gitDir(cmd.Context())
		if e != nil {
			return e
		}
		if kind == "" {
			kind = "pre-push"
		}
		return hooks.Uninstall(d, kind)
	}}
	install.Flags().StringVar(&kind, "type", "", "pre-commit or pre-push")
	install.Flags().BoolVar(&blocking, "blocking", false, "fail the Git operation when deterministic analysis fails")
	uninstall.Flags().StringVar(&kind, "type", "", "pre-commit or pre-push")
	h.AddCommand(install, uninstall)
	return h
}
func gitDir(ctx context.Context) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--absolute-git-dir")
	b, e := cmd.Output()
	return strings.TrimSpace(string(b)), e
}
func githubCmd(f *flags) *cobra.Command {
	g := &cobra.Command{Use: "github", Short: "GitHub integration"}
	var pr int
	var repo string
	var existing bool
	c := &cobra.Command{Use: "comment", RunE: func(cmd *cobra.Command, args []string) error {
		if !existing {
			if e := analyze(cmd.Context(), *f); e != nil {
				return e
			}
		}
		out := f.output
		if out == "" {
			out = ".impactr"
		}
		if existing {
			if e := artifact.Validate(out); e != nil {
				return e
			}
		}
		return integrations.GitHubComment(cmd.Context(), integrations.ExecRunner{}, pr, repo, filepath.Join(out, "summary.md"), artifactFiles(out))
	}}
	c.Flags().IntVar(&pr, "pr", envInt("GITHUB_PR_NUMBER"), "pull request number")
	c.Flags().StringVar(&repo, "repo", os.Getenv("GITHUB_REPOSITORY"), "owner/repository")
	c.Flags().BoolVar(&existing, "existing", false, "publish already-generated, validated artifacts")
	c.Flags().StringVar(&f.output, "output", "", "artifact directory (default .impactr)")
	g.AddCommand(c)
	return g
}
func gitlabCmd(f *flags) *cobra.Command {
	g := &cobra.Command{Use: "gitlab", Short: "GitLab integration"}
	var mr int
	var project, baseURL string
	c := &cobra.Command{Use: "comment", RunE: func(cmd *cobra.Command, args []string) error {
		if e := analyze(cmd.Context(), *f); e != nil {
			return e
		}
		out := f.output
		if out == "" {
			out = ".impactr"
		}
		b, e := os.ReadFile(filepath.Join(out, "summary.md"))
		if e != nil {
			return e
		}
		client := integrations.GitLab{BaseURL: baseURL, Project: project, Token: os.Getenv("GITLAB_TOKEN")}
		return client.Comment(cmd.Context(), mr, string(b), artifactFiles(out))
	}}
	c.Flags().IntVar(&mr, "mr", envInt("CI_MERGE_REQUEST_IID"), "merge request IID")
	c.Flags().StringVar(&project, "project", os.Getenv("CI_PROJECT_ID"), "project ID or path")
	c.Flags().StringVar(&baseURL, "url", os.Getenv("CI_SERVER_URL"), "GitLab base URL")
	c.Flags().StringVar(&f.output, "output", "", "artifact directory (default .impactr)")
	g.AddCommand(c)
	return g
}
func envInt(k string) int { n, _ := strconv.Atoi(os.Getenv(k)); return n }
func artifactFiles(out string) []string {
	imgs := []string{}
	if p := filepath.Join(out, "impact.png"); fileExists(p) {
		imgs = append(imgs, p)
	}
	for i := 1; ; i++ {
		p := filepath.Join(out, fmt.Sprintf("impact-%d.png", i))
		if !fileExists(p) {
			break
		}
		imgs = append(imgs, p)
	}
	return imgs
}
func fileExists(p string) bool { _, e := os.Stat(p); return e == nil }
func validateCmd() *cobra.Command {
	var dir string
	c := &cobra.Command{Use: "validate-artifact", Short: "Validate an untrusted artifact bundle", RunE: func(cmd *cobra.Command, args []string) error { return artifact.Validate(dir) }}
	c.Flags().StringVar(&dir, "dir", ".impactr", "artifact directory")
	return c
}
