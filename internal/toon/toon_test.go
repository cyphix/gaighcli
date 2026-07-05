package toon

import "testing"

func TestExtractField(t *testing.T) {
	item := map[string]any{"number": float64(42), "title": "Bug"}
	got := Extract(item, []FieldDef{Field("number", ""), Field("title", "")})
	if got["number"] != float64(42) || got["title"] != "Bug" {
		t.Fatalf("got %+v", got)
	}
}

func TestExtractPluck(t *testing.T) {
	item := map[string]any{"author": map[string]any{"login": "octocat"}}
	got := Extract(item, []FieldDef{Pluck("author", "login", "author")})
	if got["author"] != "octocat" {
		t.Fatalf("got %+v", got)
	}
}

func TestRenderList(t *testing.T) {
	items := []map[string]any{{"name": "bug"}}
	out, err := RenderList("labels", items, []FieldDef{Field("name", "")})
	if err != nil || out == "" || !contains(out, "labels") {
		t.Fatalf("out=%q err=%v", out, err)
	}
}

func TestRenderHelp(t *testing.T) {
	out := RenderHelp([]string{"Run `gai-ghcli issue list`"})
	if out == "" || !contains(out, "help[1]") {
		t.Fatalf("got %q", out)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
