package commands

import (
	"strings"
	"testing"

	"github.com/cyphix/gaighcli/internal/context"
	"github.com/cyphix/gaighcli/internal/gh"
	"github.com/cyphix/gaighcli/internal/toon"
)

func TestProjectHelp(t *testing.T) {
	out, err := Project([]string{"--help"}, nil)
	if err != nil || !strings.Contains(out, "gai-ghcli project") {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestProjectUnknownSubcommand(t *testing.T) {
	out, err := Project([]string{"nope"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Unknown project subcommand") {
		t.Fatalf("out=%q", out)
	}
}

func TestProjectListEmpty(t *testing.T) {
	mock := &gh.MockRunner{
		Response: map[string]gh.MockResponse{
			`["project","list","--format","json","--limit","30","--owner","cyphix"]`: {
				Stdout: `{"projects":[],"totalCount":0}`,
			},
		},
	}
	old := Runner
	Runner = mock
	defer func() { Runner = old }()

	out, err := Project([]string{"list"}, &context.RepoContext{
		Owner: "cyphix", NWO: "cyphix/repo", Source: context.SourceFlag,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "count: 0") {
		t.Fatalf("out=%q", out)
	}
	if !strings.Contains(out, "projects[0]") {
		t.Fatalf("expected empty projects list, out=%q", out)
	}
}

func TestProjectListOwnerDefault(t *testing.T) {
	mock := &gh.MockRunner{
		Response: map[string]gh.MockResponse{
			`["project","list","--format","json","--limit","30","--owner","myorg"]`: {
				Stdout: `{"projects":[{"number":1,"title":"Board","owner":{"login":"myorg"},"closed":false,"items":{"totalCount":3},"url":"https://github.com/orgs/myorg/projects/1"}],"totalCount":1}`,
			},
		},
	}
	old := Runner
	Runner = mock
	defer func() { Runner = old }()

	out, err := Project([]string{"list"}, &context.RepoContext{
		Owner: "myorg", NWO: "myorg/repo", Source: context.SourceGit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Board") || !strings.Contains(out, "myorg") {
		t.Fatalf("out=%q", out)
	}
	if len(mock.Calls) != 1 || mock.Calls[0][len(mock.Calls[0])-1] != "myorg" {
		t.Fatalf("calls=%v", mock.Calls)
	}
}

func TestProjectItemListFieldDefs(t *testing.T) {
	items := []map[string]any{{
		"id":     "PVTI_abc",
		"title":  "Fix login",
		"status": "Ready",
		"content": map[string]any{
			"type":       "Issue",
			"number":     float64(42),
			"repository": "owner/repo",
		},
		"labels": []any{"bug", "security"},
	}}
	out, err := toon.RenderList("items", items, projectItemListSchema)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"PVTI_abc", "Fix login", "Ready", "Issue", "42", "owner/repo", "bug,security"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in out=%q", want, out)
		}
	}
}

func TestProjectFieldListFieldDefs(t *testing.T) {
	fields := []map[string]any{{
		"id":   "PVTF_abc",
		"name": "Status",
		"type": "ProjectV2SingleSelectField",
		"options": []any{
			map[string]any{"id": "1", "name": "Backlog"},
			map[string]any{"id": "2", "name": "Done"},
		},
	}}
	out, err := toon.RenderList("fields", fields, projectFieldListSchema)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"PVTF_abc", "Status", "SingleSelectField", "Backlog,Done"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in out=%q", want, out)
		}
	}
}

func TestProjectLinkDefaultsRepo(t *testing.T) {
	mock := &gh.MockRunner{
		Response: map[string]gh.MockResponse{
			`["project","link","1","--owner","cyphix","--repo","cyphix/worldbuilder"]`: {Stdout: ""},
		},
	}
	old := Runner
	Runner = mock
	defer func() { Runner = old }()

	out, err := Project([]string{"link", "1"}, &context.RepoContext{
		Owner: "cyphix", Name: "worldbuilder", NWO: "cyphix/worldbuilder", Source: context.SourceGit,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "link: ok") {
		t.Fatalf("out=%q", out)
	}
	if len(mock.Calls) != 1 {
		t.Fatalf("calls=%v", mock.Calls)
	}
	foundRepo := false
	for i, arg := range mock.Calls[0] {
		if arg == "--repo" && i+1 < len(mock.Calls[0]) && mock.Calls[0][i+1] == "cyphix/worldbuilder" {
			foundRepo = true
		}
	}
	if !foundRepo {
		t.Fatalf("expected --repo cyphix/worldbuilder in %v", mock.Calls[0])
	}
}
