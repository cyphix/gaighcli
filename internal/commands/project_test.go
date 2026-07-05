package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cyphix/gaighcli/internal/context"
	"github.com/cyphix/gaighcli/internal/gh"
	"github.com/cyphix/gaighcli/internal/toon"
)

func testRepoCtx() *context.RepoContext {
	return &context.RepoContext{
		Owner: "cyphix", Name: "repo", NWO: "owner/repo", Source: context.SourceGit,
	}
}

func writeTestIssueBoardConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := issueBoardConfig{
		ProjectNumber: 3,
		ProjectID:     "PVT_kwHOAKKpoc4BcgFk",
		StatusFieldID: "PVTSSF_status",
		StatusOptions: map[string]string{
			"Ready": "61e4505c",
		},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "issue-board.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func sampleProjectItem(status string) map[string]any {
	return map[string]any{
		"id":     "PVTI_abc",
		"title":  "Fix login",
		"status": status,
		"content": map[string]any{
			"type":       "Issue",
			"number":     float64(42),
			"repository": "owner/repo",
			"url":        "https://github.com/owner/repo/issues/42",
		},
	}
}

func TestProjectHelp(t *testing.T) {
	out, err := Project([]string{"--help"}, nil)
	if err != nil || !strings.Contains(out, "gai-ghcli project") {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if !strings.Contains(out, "item-set-status") {
		t.Fatalf("expected item-set-status in help, out=%q", out)
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

func TestFindProjectItemByIssue(t *testing.T) {
	items := []map[string]any{sampleProjectItem("Backlog")}
	item, ok := findProjectItemByIssue(items, 42, "owner/repo")
	if !ok || item == nil {
		t.Fatal("expected match")
	}
	_, ok = findProjectItemByIssue(items, 99, "owner/repo")
	if ok {
		t.Fatal("expected no match for wrong number")
	}
	_, ok = findProjectItemByIssue(items, 42, "other/repo")
	if ok {
		t.Fatal("expected no match for wrong repo")
	}
}

func TestFindProjectItemByTitle(t *testing.T) {
	items := []map[string]any{sampleProjectItem("Backlog")}
	item, ambiguous, ok := findProjectItemByTitle(items, "Fix login", "owner/repo")
	if !ok || item == nil || len(ambiguous) != 0 {
		t.Fatalf("expected single match, ok=%v ambiguous=%v", ok, ambiguous)
	}
	_, _, ok = findProjectItemByTitle(items, "Missing", "owner/repo")
	if ok {
		t.Fatal("expected no match")
	}

	dup := []map[string]any{
		sampleProjectItem("Backlog"),
		{
			"id":     "PVTI_def",
			"title":  "Fix login",
			"status": "Ready",
			"content": map[string]any{
				"number":     float64(43),
				"repository": "owner/repo",
			},
		},
	}
	_, ambiguous, ok = findProjectItemByTitle(dup, "Fix login", "owner/repo")
	if ok || len(ambiguous) != 2 {
		t.Fatalf("expected ambiguous match, ok=%v len=%d", ok, len(ambiguous))
	}
}

func TestValidateIssueBoardConfig(t *testing.T) {
	if err := validateIssueBoardConfig(&issueBoardConfig{ProjectNumber: 3}, 3); err != nil {
		t.Fatal(err)
	}
	if err := validateIssueBoardConfig(&issueBoardConfig{ProjectNumber: 3}, 5); err == nil {
		t.Fatal("expected mismatch error")
	}
}

func TestSetProjectItemStatus(t *testing.T) {
	configPath := writeTestIssueBoardConfig(t)
	itemListKey := `["project","item-list","3","--format","json","--limit","500","--owner","cyphix"]`
	itemListBacklog := `{"items":[{"id":"PVTI_abc","title":"Fix login","status":"Backlog","content":{"type":"Issue","number":42,"repository":"owner/repo","url":"https://github.com/owner/repo/issues/42"}}],"totalCount":1}`
	itemListReady := strings.Replace(itemListBacklog, `"Backlog"`, `"Ready"`, 1)
	itemEditKey := `["project","item-edit","--format","json","--id","PVTI_abc","--project-id","PVT_kwHOAKKpoc4BcgFk","--field-id","PVTSSF_status","--single-select-option-id","61e4505c"]`
	fieldListKey := `["project","field-list","3","--format","json","--limit","100","--owner","cyphix"]`
	projectViewKey := `["project","view","3","--format","json","--owner","cyphix"]`
	fieldListResp := `{"fields":[{"id":"PVTSSF_status","name":"Status","type":"ProjectV2SingleSelectField","options":[{"id":"opt1","name":"Backlog"},{"id":"opt2","name":"Ready"},{"id":"opt3","name":"Done"}]}],"totalCount":1}`

	tests := []struct {
		name      string
		args      []string
		mock      map[string]gh.MockResponse
		wantErr   string
		wantOut   []string
		noItemEdit bool
	}{
		{
			name: "success by number",
			args: []string{"item-set-status", "3", "--issue", "42", "--status", "Ready", "--config", configPath},
			mock: map[string]gh.MockResponse{
				itemListKey: {Stdout: itemListBacklog},
				itemEditKey: {Stdout: `{"id":"PVTI_abc"}`},
			},
			wantOut: []string{"issue: 42", "changed: yes", "status: Ready", "previousStatus: Backlog", "help["},
		},
		{
			name: "success by title",
			args: []string{"item-set-status", "3", "--title", "Fix login", "--status", "Ready", "--config", configPath},
			mock: map[string]gh.MockResponse{
				itemListKey: {Stdout: itemListBacklog},
				itemEditKey: {Stdout: `{"id":"PVTI_abc"}`},
			},
			wantOut: []string{"title: Fix login", "issue: 42", "changed: yes", "help["},
		},
		{
			name: "no-op",
			args: []string{"item-set-status", "3", "--issue", "42", "--status", "Ready", "--config", configPath},
			mock: map[string]gh.MockResponse{
				itemListKey: {Stdout: itemListReady},
			},
			wantOut:      []string{"changed: no", "status: Ready", "help["},
			noItemEdit:   true,
		},
		{
			name: "issue not found",
			args: []string{"item-set-status", "3", "--issue", "42", "--status", "Ready", "--config", configPath},
			mock: map[string]gh.MockResponse{
				itemListKey: {Stdout: `{"items":[],"totalCount":0}`},
			},
			wantErr: "not found on project",
		},
		{
			name: "title not found",
			args: []string{"item-set-status", "3", "--title", "Missing issue", "--status", "Ready", "--config", configPath},
			mock: map[string]gh.MockResponse{
				itemListKey: {Stdout: itemListBacklog},
			},
			wantErr: "No project item titled",
		},
		{
			name: "ambiguous title",
			args: []string{"item-set-status", "3", "--title", "Fix login", "--status", "Ready", "--config", configPath},
			mock: map[string]gh.MockResponse{
				itemListKey: {Stdout: `{"items":[{"id":"PVTI_abc","title":"Fix login","status":"Backlog","content":{"number":42,"repository":"owner/repo"}},{"id":"PVTI_def","title":"Fix login","status":"Ready","content":{"number":43,"repository":"owner/repo"}}],"totalCount":2}`},
			},
			wantErr: "Multiple items match",
		},
		{
			name: "unknown status",
			args: []string{"item-set-status", "3", "--issue", "42", "--status", "Foo"},
			mock: map[string]gh.MockResponse{
				itemListKey:    {Stdout: itemListBacklog},
				projectViewKey: {Stdout: `{"id":"PVT_kwHOAKKpoc4BcgFk"}`},
				fieldListKey:   {Stdout: fieldListResp},
			},
			wantErr: "Unknown status",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := &gh.MockRunner{Response: tc.mock}
			old := Runner
			Runner = mock
			defer func() { Runner = old }()

			out, err := Project(tc.args, testRepoCtx())
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got out=%q", tc.wantErr, out)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err=%q want substring %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.wantOut {
				if !strings.Contains(out, want) {
					t.Fatalf("missing %q in out=%q", want, out)
				}
			}
			if strings.Contains(out, "{") {
				t.Fatalf("output should be TOON not raw JSON: %q", out)
			}
			itemEditCalled := false
			for _, call := range mock.Calls {
				if len(call) >= 2 && call[0] == "project" && call[1] == "item-edit" {
					itemEditCalled = true
				}
			}
			if tc.noItemEdit && itemEditCalled {
				t.Fatal("item-edit should not have been called")
			}
			if !tc.noItemEdit && tc.wantErr == "" && !itemEditCalled && tc.name != "unknown status" {
				// success cases should call item-edit unless no-op
				if tc.name != "no-op" {
					t.Fatal("expected item-edit to be called")
				}
			}
		})
	}
}
