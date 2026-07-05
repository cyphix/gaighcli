package errors

import (
	"testing"

	"github.com/cyphix/gaisdk"
)

func TestMapGhErrorRepoNotFound(t *testing.T) {
	err := MapGhError("Could not resolve to a Repository with the name 'cli/cli'", 1)
	if err.Code != "REPO_NOT_FOUND" || !contains(err.Message, "cli/cli") {
		t.Fatalf("got %+v", err)
	}
}

func TestMapGhErrorIssueNotFound(t *testing.T) {
	err := MapGhError("issue 42 not found", 1)
	if err.Code != "NOT_FOUND" || !contains(err.Message, "42") {
		t.Fatalf("got %+v", err)
	}
}

func TestMapGhErrorAuth(t *testing.T) {
	err := MapGhError("To get started, please run: gh auth login", 1)
	if err.Code != "AUTH_REQUIRED" {
		t.Fatalf("got %+v", err)
	}
}

func TestMapGhErrorGenericNotFound(t *testing.T) {
	err := MapGhError("something not found here", 1)
	if err.Code != "NOT_FOUND" {
		t.Fatalf("got %+v", err)
	}
}

func TestGhNotInstalled(t *testing.T) {
	err := GhNotInstalled()
	if err.Code != "GH_NOT_INSTALLED" {
		t.Fatalf("got %+v", err)
	}
}

func TestExitCodeValidation(t *testing.T) {
	if gaisdk.ExitCodeForError(NewGoAIError("x", "VALIDATION_ERROR")) != 2 {
		t.Fatal("expected 2")
	}
	if gaisdk.ExitCodeForError(NewGoAIError("x", "UNKNOWN")) != 1 {
		t.Fatal("expected 1")
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
