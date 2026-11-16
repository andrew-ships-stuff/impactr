package integrations

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
)

type CommandRunner interface {
	Run(context.Context, string, ...string) error
}
type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if out, e := cmd.CombinedOutput(); e != nil {
		return fmt.Errorf("%s: %w: %s", name, e, out)
	}
	return nil
}
func GitHubComment(ctx context.Context, r CommandRunner, pr int, repo, bodyFile, image string) error {
	if pr < 1 {
		return fmt.Errorf("invalid pull request number")
	}
	args := []string{"pr", "comment", strconv.Itoa(pr), "--body-file", bodyFile, "--edit-last", "--create-if-none"}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	if image != "" {
		args = append(args, "--attach", image+"#Impactr architecture impact diagram")
	}
	return r.Run(ctx, "gh", args...)
}
