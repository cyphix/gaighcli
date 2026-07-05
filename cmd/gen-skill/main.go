package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cyphix/gaighcli/internal/skill"
)

func main() {
	check := flag.Bool("check", false, "exit 1 if skills/gai-ghcli/SKILL.md is out of date")
	out := flag.String("o", skill.DefaultSkillPath, "output path for SKILL.md")
	flag.Parse()

	content, err := skill.CreateSkillMarkdown()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gen-skill: %v\n", err)
		os.Exit(1)
	}

	if *check {
		actual, err := os.ReadFile(*out)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s is missing or unreadable. Run `go run ./cmd/gen-skill` and commit the result.\n", *out)
			os.Exit(1)
		}
		if string(actual) != content {
			fmt.Fprintf(os.Stderr, "%s is out of date. Run `go run ./cmd/gen-skill` and commit the result.\n", *out)
			os.Exit(1)
		}
		fmt.Printf("%s is up to date.\n", *out)
		return
	}

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "gen-skill: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, []byte(content), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "gen-skill: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %s\n", *out)
}
