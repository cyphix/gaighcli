package commands

import (
	"strings"
	"testing"

	"github.com/cyphix/gaighcli/internal/context"
	"github.com/cyphix/gaighcli/internal/gh"
)

func TestLabelHelp(t *testing.T) {
	out, err := Label([]string{"--help"}, nil)
	if err != nil || !strings.Contains(out, "gai-ghcli label") {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestLabelListEmpty(t *testing.T) {
	mock := &gh.MockRunner{
		Response: map[string]gh.MockResponse{
			`["label","list","--json","name","--limit","500"]`: {Stdout: "[]"},
		},
	}
	old := Runner
	Runner = mock
	defer func() { Runner = old }()

	out, err := Label([]string{"list"}, &context.RepoContext{NWO: "o/r", Source: context.SourceFlag})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "count: 0") {
		t.Fatalf("out=%q", out)
	}
}

func TestLabelCreateIdempotent(t *testing.T) {
	mock := &gh.MockRunner{
		Response: map[string]gh.MockResponse{
			`["label","list","--json","name"]`: {Stdout: `[{"name":"bug"}]`},
		},
	}
	old := Runner
	Runner = mock
	defer func() { Runner = old }()

	out, err := Label([]string{"create", "--name", "bug", "--color", "ff0000"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "already_exists") {
		t.Fatalf("out=%q", out)
	}
}

func TestSecretRejectsBodyFlag(t *testing.T) {
	_, err := Secret([]string{"set", "KEY", "--body", "val"}, nil)
	if err == nil || !strings.Contains(err.Error(), "stdin") {
		t.Fatalf("err=%v", err)
	}
}

func TestIssueUnknownSubcommand(t *testing.T) {
	out, err := Issue([]string{"nope"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Unknown issue subcommand") {
		t.Fatalf("out=%q", out)
	}
}

func TestApiHelp(t *testing.T) {
	out, err := Api([]string{"--help"}, nil)
	if err != nil || !strings.Contains(out, "gai-ghcli api") {
		t.Fatalf("out=%q err=%v", out, err)
	}
}
