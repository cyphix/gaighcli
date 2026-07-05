package skill

import (
	"os"
	"strings"
	"testing"
)

func TestExtractCommandsBlock(t *testing.T) {
	block, err := ExtractCommandsBlock()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(block, "commands[14]:") {
		t.Fatalf("unexpected block: %q", block)
	}
	if !strings.Contains(block, "issue, pr, run") || !strings.Contains(block, "project") {
		t.Fatalf("missing commands: %q", block)
	}
}

func TestCreateSkillMarkdown(t *testing.T) {
	md, err := CreateSkillMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"name: gai-ghcli",
		"user-invocable: false",
		"Ken Walker (cyphix)",
		"go install github.com/cyphix/gaighcli/cmd/gai-ghcli@latest",
		"commands[14]:",
		"kunchenguid",
		"## Availability",
		"command -v gai-ghcli",
		"Fall back to `gh`",
		"Warn the user once per session",
		"when it is installed",
	} {
		if !strings.Contains(md, want) {
			t.Fatalf("missing %q in generated skill", want)
		}
	}
}

func TestCommittedSkillIsUpToDate(t *testing.T) {
	expected, err := CreateSkillMarkdown()
	if err != nil {
		t.Fatal(err)
	}
	actual, err := os.ReadFile(DefaultSkillPath)
	if err != nil {
		t.Skipf("committed skill not present yet: %v", err)
	}
	if string(actual) != expected {
		t.Fatalf("%s is out of date; run `go run ./cmd/gen-skill`", DefaultSkillPath)
	}
}
