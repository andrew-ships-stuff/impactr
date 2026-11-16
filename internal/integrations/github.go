package integrations

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
	return exec.CommandContext(ctx, name, args...).Output()
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

func GitHubComment(ctx context.Context, r CommandRunner, pr int, repo, bodyFile, image string) error {
	if pr < 1 {
		return fmt.Errorf("invalid pull request number")
	}
	mode := "edit"
	body, re := os.ReadFile(bodyFile)
	if re == nil {
		if next, ok := ParseStats(string(body)); ok {
			if qr, rok := r.(OutputRunner); rok {
				if prev, found := lastStats(ctx, qr, repo, pr); found && prev.substantial(next) {
					mode = "new"
				}
			}
		}
	}
	args := []string{"pr", "comment", strconv.Itoa(pr)}
	cleanup := func() error { return nil }
	switch mode {
	case "edit":
		args = append(args, "--body-file", bodyFile, "--edit-last", "--create-if-none")
	case "new":
		tmp, te := os.CreateTemp("", "impactr-comment-*.md")
		if te != nil {
			return te
		}
		if _, te = tmp.WriteString(supersedeNote + string(body)); te != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			return te
		}
		te = tmp.Close()
		name := tmp.Name()
		cleanup = func() error { return os.Remove(name) }
		args = append(args, "--body-file", name)
	}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	if image != "" {
		args = append(args, "--attach", image+"#Impactr architecture impact diagram")
	}
	if re := r.Run(ctx, "gh", args...); re != nil {
		_ = cleanup()
		return re
	}
	return cleanup()
}
