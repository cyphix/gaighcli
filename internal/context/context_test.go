package context

import (
	"os"
	"testing"
)

func TestResolveRepoFlag(t *testing.T) {
	r := ResolveRepo("cli/cli")
	if r == nil {
		t.Fatal("expected repo")
	}
	if r.NWO != "cli/cli" || r.Source != SourceFlag {
		t.Fatalf("got %+v", r)
	}
}

func TestResolveRepoInvalidFlag(t *testing.T) {
	for _, nwo := range []string{"invalid", "a/b/c", "/name", "owner/"} {
		if r := ResolveRepo(nwo); r != nil {
			t.Fatalf("expected nil for %q, got %+v", nwo, r)
		}
	}
}

func TestResolveRepoEnv(t *testing.T) {
	t.Setenv("GH_REPO", "octocat/hello-world")
	r := ResolveRepo("")
	if r == nil || r.NWO != "octocat/hello-world" || r.Source != SourceEnv {
		t.Fatalf("got %+v", r)
	}
}

func TestResolveRepoFlagPriority(t *testing.T) {
	t.Setenv("GH_REPO", "env-owner/env-repo")
	r := ResolveRepo("flag-owner/flag-repo")
	if r == nil || r.Source != SourceFlag || r.NWO != "flag-owner/flag-repo" {
		t.Fatalf("got %+v", r)
	}
}

func TestParseRemoteURL(t *testing.T) {
	tests := []struct {
		url      string
		wantNWO  string
		wantNil  bool
	}{
		{"git@github.com:cli/cli.git", "cli/cli", false},
		{"git@github.com:owner/repo", "owner/repo", false},
		{"https://github.com/cli/cli.git", "cli/cli", false},
		{"https://github.com/owner/repo", "owner/repo", false},
		{"https://gitlab.com/owner/repo", "", true},
	}
	for _, tc := range tests {
		r := parseRemoteURL(tc.url)
		if tc.wantNil {
			if r != nil {
				t.Errorf("url %q: expected nil, got %+v", tc.url, r)
			}
			continue
		}
		if r == nil || r.NWO != tc.wantNWO || r.Source != SourceGit {
			t.Errorf("url %q: got %+v", tc.url, r)
		}
	}
}

func TestResolveRepoNoSource(t *testing.T) {
	os.Unsetenv("GH_REPO")
	// Without git repo this may return nil - just ensure no panic
	_ = ResolveRepo("")
}
