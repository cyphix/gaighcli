package args

import (
	"testing"
)

func TestGetFlag(t *testing.T) {
	args := []string{"--repo", "cli/cli", "--state", "open"}
	if got := GetFlag(args, "--repo"); got != "cli/cli" {
		t.Fatalf("got %q", got)
	}
	if got := GetFlag([]string{"--repo=cli/cli", "--state", "open"}, "--repo"); got != "cli/cli" {
		t.Fatalf("got %q", got)
	}
	if got := GetFlag([]string{"--state", "open"}, "--repo"); got != "" {
		t.Fatalf("expected empty")
	}
}

func TestTakeFlag(t *testing.T) {
	args := []string{"--repo", "cli/cli", "--state", "open"}
	if got := TakeFlag(&args, "--repo"); got != "cli/cli" {
		t.Fatalf("got %q", got)
	}
	if len(args) != 2 || args[0] != "--state" {
		t.Fatalf("args = %v", args)
	}
}

func TestHasFlag(t *testing.T) {
	if !HasFlag([]string{"--full", "--json"}, "--full") {
		t.Fatal("expected true")
	}
	if HasFlag([]string{"--json"}, "--full") {
		t.Fatal("expected false")
	}
}

func TestTakeBoolFlag(t *testing.T) {
	args := []string{"--full", "--json"}
	if !TakeBoolFlag(&args, "--full") {
		t.Fatal("expected true")
	}
	if len(args) != 1 {
		t.Fatalf("args = %v", args)
	}
}

func TestGetAllFlags(t *testing.T) {
	got := GetAllFlags([]string{"--label", "bug", "--label", "urgent"}, "--label")
	if len(got) != 2 || got[0] != "bug" || got[1] != "urgent" {
		t.Fatalf("got %v", got)
	}
}

func TestGetPositional(t *testing.T) {
	if got := GetPositional([]string{"list", "42", "--comments"}, 0); got != "list" {
		t.Fatalf("got %q", got)
	}
}

func TestRequireNumber(t *testing.T) {
	n, err := RequireNumber("42", "issue")
	if err != nil || n != 42 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if _, err := RequireNumber("", "issue"); err == nil {
		t.Fatal("expected error")
	}
}

func TestTakeNumber(t *testing.T) {
	args := []string{"view", "42", "--comments"}
	n, err := TakeNumber(&args, "issue")
	if err != nil || n != 42 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	if len(args) != 2 {
		t.Fatalf("args = %v", args)
	}
}
