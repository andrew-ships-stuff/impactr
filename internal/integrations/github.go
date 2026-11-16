package integrations

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type CommandRunner interface {
	Run(context.Context, string, ...string) error
}
type OutputRunner interface {
	Output(context.Context, string, ...string) ([]byte, error)
}
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if out, e := cmd.CombinedOutput(); e != nil {
		return fmt.Errorf("%s: %w: %s", name, e, out)
	}
	return nil
}
func (ExecRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var errs bytes.Buffer
	cmd.Stderr = &errs
	out, e := cmd.Output()
	if e != nil {
		return out, fmt.Errorf("%s %v: %w: %s", name, args, e, strings.TrimSpace(errs.String()))
	}
	return out, nil
}

// Stats is the machine-readable summary embedded in every Impactr comment so
// republishing can decide between editing the existing comment and posting a
// new one when the analysis changed substantially.
type Stats struct{ Nodes, Edges, Affected int }

var statsRE = regexp.MustCompile(`impactr-stats:v1 nodes=(\d+) edges=(\d+) affected=(\d+)`)

func ParseStats(body string) (Stats, bool) {
	m := statsRE.FindStringSubmatch(body)
	if m == nil {
		return Stats{}, false
	}
	n, _ := strconv.Atoi(m[1])
	e, _ := strconv.Atoi(m[2])
	a, _ := strconv.Atoi(m[3])
	return Stats{Nodes: n, Edges: e, Affected: a}, true
}

// substantial reports whether any metric more than doubled in either
// direction (with a slack of 5 to absorb trivial churn).
func (s Stats) substantial(o Stats) bool {
	return jumped(s.Nodes, o.Nodes) || jumped(s.Edges, o.Edges) || jumped(s.Affected, o.Affected)
}
func jumped(prev, next int) bool { return next > 2*prev+5 || prev > 2*next+5 }

const supersedeNote = "> Supersedes an earlier Impactr analysis: the measured impact changed substantially since the last comment.\n\n"

// lastStats returns the stats of the most recent Impactr comment on the PR.
func lastStats(ctx context.Context, r OutputRunner, repo string, pr int) (Stats, bool) {
	if repo == "" {
		return Stats{}, false
	}
	out, e := r.Output(ctx, "gh", "api", "--paginate",
		"--jq", `.[] | .body | capture("impactr-stats:v1 nodes=(?<n>[0-9]+) edges=(?<e>[0-9]+) affected=(?<a>[0-9]+)") | "\(.n) \(.e) \(.a)"`,
		fmt.Sprintf("repos/%s/issues/%d/comments", repo, pr))
	if e != nil {
		return Stats{}, false
	}
	last := ""
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			last = line
		}
	}
	if last == "" {
		return Stats{}, false
	}
	var n, e2, a int
	if _, err := fmt.Sscanf(last, "%d %d %d", &n, &e2, &a); err != nil {
		return Stats{}, false
	}
	return Stats{Nodes: n, Edges: e2, Affected: a}, true
}

// uploadAsset posts the file to the repository uploads endpoint and returns
// its durable asset URL. Unlike gh pr comment --attach (which demands a
// classic repo-scoped token), this works with a GITHUB_TOKEN that has
// contents: write.
func uploadAsset(ctx context.Context, r OutputRunner, repo, path string) (string, bool) {
	if repo == "" {
		return "", false
	}
	out, e := r.Output(ctx, "gh", "api", "--method", "POST",
		fmt.Sprintf("repos/%s/uploads", repo),
		"-F", "name="+filepath.Base(path),
		"-F", "file=@"+path)
	if e != nil {
		fmt.Fprintf(os.Stderr, "impactr: uploads endpoint failed: %v\n", e)
		return "", false
	}
	var v struct {
		AssetURL string `json:"asset_url"`
	}
	if json.Unmarshal(out, &v) != nil || v.AssetURL == "" {
		fmt.Fprintf(os.Stderr, "impactr: uploads endpoint returned no asset_url: %s\n", out)
		return "", false
	}
	return v.AssetURL, true
}

// commitAsset stores the diagram on a dedicated assets branch through the
// Contents API and returns its raw URL. This is the fallback for GITHUB_TOKEN,
// which the uploads endpoint rejects with 404.
func commitAsset(ctx context.Context, r OutputRunner, repo string, pr int, path, suffix string) (string, bool) {
	if repo == "" {
		return "", false
	}
	data, e := os.ReadFile(path)
	if e != nil {
		return "", false
	}
	name := fmt.Sprintf("pr-%d%s.png", pr, suffix)
	branch, p := "impactr-assets", "impactr/"+name
	payload := map[string]any{
		"content": base64.StdEncoding.EncodeToString(data),
		"branch":  branch,
		"message": fmt.Sprintf("impactr: update impact diagram for PR %d", pr),
	}
	if sha, ok := contentsSHA(ctx, r, repo, p, branch); ok {
		payload["sha"] = sha
	}
	body, e := json.Marshal(payload)
	if e != nil {
		return "", false
	}
	tmp, e := os.CreateTemp("", "impactr-asset-*.json")
	if e != nil {
		return "", false
	}
	defer os.Remove(tmp.Name())
	if _, e = tmp.Write(body); e != nil {
		tmp.Close()
		return "", false
	}
	tmp.Close()
	_, e = r.Output(ctx, "gh", "api", "--method", "PUT", "repos/"+repo+"/contents/"+p, "--input", tmp.Name())
	if e != nil && strings.Contains(e.Error(), "not found") {
		if !createBranch(ctx, r, repo, branch) {
			fmt.Fprintf(os.Stderr, "impactr: contents API failed: %v\n", e)
			return "", false
		}
		if _, e = r.Output(ctx, "gh", "api", "--method", "PUT", "repos/"+repo+"/contents/"+p, "--input", tmp.Name()); e != nil {
			fmt.Fprintf(os.Stderr, "impactr: contents API failed after branch create: %v\n", e)
			return "", false
		}
		return "https://raw.githubusercontent.com/" + repo + "/" + branch + "/" + p, true
	}
	if e != nil {
		fmt.Fprintf(os.Stderr, "impactr: contents API failed: %v\n", e)
		return "", false
	}
	return "https://raw.githubusercontent.com/" + repo + "/" + branch + "/" + p, true
}

func createBranch(ctx context.Context, r OutputRunner, repo, branch string) bool {
	def, e := r.Output(ctx, "gh", "api", "repos/"+repo, "--jq", ".default_branch")
	if e != nil {
		return false
	}
	sha, e := r.Output(ctx, "gh", "api", fmt.Sprintf("repos/%s/commits/%s", repo, strings.TrimSpace(string(def))), "--jq", ".sha")
	if e != nil {
		return false
	}
	_, e = r.Output(ctx, "gh", "api", "--method", "POST", "repos/"+repo+"/git/refs",
		"-f", "ref=refs/heads/"+branch,
		"-f", "sha="+strings.TrimSpace(string(sha)))
	return e == nil
}

func contentsSHA(ctx context.Context, r OutputRunner, repo, p, branch string) (string, bool) {
	out, e := r.Output(ctx, "gh", "api", "repos/"+repo+"/contents/"+p, "--jq", ".sha", "-f", "ref="+branch)
	if e != nil {
		return "", false
	}
	sha := strings.TrimSpace(string(out))
	if sha == "" || sha == "null" {
		return "", false
	}
	return sha, true
}

func GitHubComment(ctx context.Context, r CommandRunner, pr int, repo, bodyFile string, images []string) error {
	if pr < 1 {
		return fmt.Errorf("invalid pull request number")
	}
	mode := "edit"
	changed := false
	body, re := os.ReadFile(bodyFile)
	if re == nil {
		if next, ok := ParseStats(string(body)); ok {
			if qr, rok := r.(OutputRunner); rok {
				if prev, found := lastStats(ctx, qr, repo, pr); found && prev.substantial(next) {
					mode = "new"
					body = append([]byte(supersedeNote), body...)
					changed = true
				}
			}
		}
	}
	attach := []string{}
	if len(images) > 0 {
		uploaded := false
		if qr, rok := r.(OutputRunner); rok && repo != "" {
			uploaded = true
			for i, image := range images {
				url, ok := uploadAsset(ctx, qr, repo, image)
				if !ok {
					suffix := ""
					if i > 0 {
						suffix = "-" + strconv.Itoa(i)
					}
					url, ok = commitAsset(ctx, qr, repo, pr, image, suffix)
				}
				if !ok {
					uploaded = false
					break
				}
				body = []byte(strings.ReplaceAll(string(body), "]("+filepath.Base(image)+")", "]("+url+")"))
				changed = true
			}
		}
		if !uploaded {
			for _, image := range images {
				attach = append(attach, image+"#Impactr architecture impact diagram")
			}
		}
	}
	args := []string{"pr", "comment", strconv.Itoa(pr)}
	var bf string
	var cleanup func() error = func() error { return nil }
	write := func(content []byte) (string, func() error, error) {
		tmp, te := os.CreateTemp("", "impactr-comment-*.md")
		if te != nil {
			return "", cleanup, te
		}
		if _, te = tmp.Write(content); te != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			return "", cleanup, te
		}
		return tmp.Name(), func() error { return os.Remove(tmp.Name()) }, tmp.Close()
	}
	if changed {
		var te error
		bf, cleanup, te = write(body)
		if te != nil {
			return te
		}
	} else {
		bf = bodyFile
	}
	if mode == "edit" {
		args = append(args, "--body-file", bf, "--edit-last", "--create-if-none")
	} else {
		args = append(args, "--body-file", bf)
	}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	for _, a := range attach {
		args = append(args, "--attach", a)
	}
	err := r.Run(ctx, "gh", args...)
	if err != nil && len(attach) > 0 {
		fmt.Fprintf(os.Stderr, "impactr: comment with attached images failed (%v); retrying without images\n", err)
		_ = cleanup()
		body = []byte(stripImages(string(body)))
		changed = true
		var te error
		bf, cleanup, te = write(body)
		if te != nil {
			return te
		}
		args = []string{"pr", "comment", strconv.Itoa(pr), "--body-file", bf}
		if mode == "edit" {
			args = append(args, "--edit-last", "--create-if-none")
		}
		if repo != "" {
			args = append(args, "--repo", repo)
		}
		err = r.Run(ctx, "gh", args...)
	}
	if err != nil {
		_ = cleanup()
		return err
	}
	return cleanup()
}

func stripImages(body string) string {
	re := regexp.MustCompile(`(?m)^\s*!\[[^\]]*\]\([^)]*\)\s*\n?`)
	return re.ReplaceAllString(body, "")
}
