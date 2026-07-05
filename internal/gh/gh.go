package gh

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/cyphix/gaighcli/internal/context"
	"github.com/cyphix/gaighcli/internal/errors"
)

const maxBufferBytes = 10 * 1024 * 1024

// ExecResult holds raw gh subprocess output.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Runner executes gh commands.
type Runner interface {
	Run(args []string, ctx *context.RepoContext) (ExecResult, error)
	RunWithStdin(args []string, input string, ctx *context.RepoContext) (ExecResult, error)
}

// Client wraps the gh CLI.
type Client struct{}

// Default is the package-level gh client.
var Default Runner = &Client{}

func buildArgs(args []string, ctx *context.RepoContext) []string {
	out := append([]string{}, args...)
	if ctx != nil && ctx.Source != context.SourceGit {
		out = append(out, "--repo", ctx.NWO)
	}
	return out
}

func (c *Client) Run(args []string, ctx *context.RepoContext) (ExecResult, error) {
	return run(buildArgs(args, ctx), "")
}

func (c *Client) RunWithStdin(args []string, input string, ctx *context.RepoContext) (ExecResult, error) {
	return run(buildArgs(args, ctx), input)
}

func run(args []string, stdin string) (ExecResult, error) {
	cmd := exec.Command("gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if stdin != "" {
		cmd.Stdin = bytes.NewBufferString(stdin)
	}
	err := cmd.Run()
	result := ExecResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if err != nil {
		if execErr, ok := err.(*exec.Error); ok && execErr.Err.Error() == "executable file not found in $PATH" {
			return ExecResult{Stderr: "ENOENT", ExitCode: 127}, nil
		}
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		result.ExitCode = 1
		return result, nil
	}
	return result, nil
}

func checkResult(result ExecResult) error {
	if result.Stderr == "ENOENT" {
		return errors.GhNotInstalled()
	}
	if result.ExitCode != 0 {
		return errors.MapGhError(result.Stderr, result.ExitCode)
	}
	return nil
}

// JSON executes gh and unmarshals JSON stdout.
func JSON[T any](runner Runner, args []string, ctx *context.RepoContext) (T, error) {
	var zero T
	result, err := runner.Run(args, ctx)
	if err != nil {
		return zero, err
	}
	if err := checkResult(result); err != nil {
		return zero, err
	}
	var out T
	if err := json.Unmarshal([]byte(result.Stdout), &out); err != nil {
		return zero, errors.NewGoAIError(
			fmt.Sprintf("Unexpected gh output: %.200s", result.Stdout),
			"UNKNOWN",
		)
	}
	return out, nil
}

// Exec executes gh and returns raw stdout.
func Exec(runner Runner, args []string, ctx *context.RepoContext) (string, error) {
	result, err := runner.Run(args, ctx)
	if err != nil {
		return "", err
	}
	if err := checkResult(result); err != nil {
		return "", err
	}
	return result.Stdout, nil
}

// Raw executes gh without throwing on non-zero exit.
func Raw(runner Runner, args []string, ctx *context.RepoContext) (ExecResult, error) {
	result, err := runner.Run(args, ctx)
	if err != nil {
		return ExecResult{}, err
	}
	if result.Stderr == "ENOENT" {
		return result, errors.GhNotInstalled()
	}
	return result, nil
}

// ExecWithStdin executes gh writing input to the child stdin.
func ExecWithStdin(runner Runner, args []string, input string, ctx *context.RepoContext) (string, error) {
	result, err := runner.RunWithStdin(args, input, ctx)
	if err != nil {
		return "", err
	}
	if err := checkResult(result); err != nil {
		return "", err
	}
	return result.Stdout, nil
}
