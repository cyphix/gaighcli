package body

import (
	"strings"
	"testing"
)

func TestCleanBody(t *testing.T) {
	in := "See [PR](https://github.com/o/r/pull/42) and ![img](http://x.com/y.png)"
	out := CleanBody(in)
	if out == in {
		t.Fatalf("expected cleanup, got %q", out)
	}
	if !contains(out, "PR#42") {
		t.Fatalf("got %q", out)
	}
}

func TestTruncateBodyShort(t *testing.T) {
	got := TruncateBody("hello", 500)
	if got != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestTruncateBodyLong(t *testing.T) {
	long := strings.Repeat("x", 600)
	got := TruncateBody(long, 500)
	if !contains(got, "truncated") {
		t.Fatalf("got %q", got)
	}
}

func TestTakeBodyRequired(t *testing.T) {
	args := []string{"--body", "hello"}
	got, err := TakeBody(&args, TakeBodyOptions{Required: true})
	if err != nil || got != "hello" {
		t.Fatalf("got %q err=%v args=%v", got, err, args)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && searchSub(s, sub)
}

func searchSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
