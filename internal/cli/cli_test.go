package cli

import (
	"strings"
	"testing"

	"github.com/cyphix/gaisdk"
)

func TestParseRepoContextArgs(t *testing.T) {
	r := parseRepoContextArgs("issue", []string{"list", "-R", "o/r", "--state", "open"})
	if r.repoFlag != "o/r" {
		t.Fatalf("repoFlag=%q", r.repoFlag)
	}
	if len(r.strippedArgs) != 3 || r.strippedArgs[0] != "list" {
		t.Fatalf("stripped=%v", r.strippedArgs)
	}
}

func TestParseRepoContextArgsSearchKeepsRepo(t *testing.T) {
	r := parseRepoContextArgs("search", []string{"issues", "q", "--repo", "o/r"})
	if r.repoFlag != "o/r" {
		t.Fatalf("repoFlag=%q", r.repoFlag)
	}
	if len(r.strippedArgs) != 4 {
		t.Fatalf("stripped=%v", r.strippedArgs)
	}
}

func TestGetCommandHelp(t *testing.T) {
	help, ok := GetCommandHelp("issue")
	if !ok || !strings.Contains(help, "gai-ghcli issue") {
		t.Fatalf("help=%q ok=%v", help, ok)
	}
	help, ok = GetCommandHelp("project")
	if !ok || !strings.Contains(help, "gai-ghcli project") {
		t.Fatalf("help=%q ok=%v", help, ok)
	}
}

func TestResolveRepoContext(t *testing.T) {
	ctx, err := ResolveRepoContext(gaisdk.ResolveContextInput{
		Command: "issue",
		Args:    []string{"list", "-R", "cli/cli"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ctx == nil || ctx.NWO != "cli/cli" {
		t.Fatalf("ctx=%+v", ctx)
	}
}
