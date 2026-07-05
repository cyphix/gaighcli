package context

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Source indicates how the target repository was resolved.
type Source string

const (
	SourceFlag Source = "flag"
	SourceEnv  Source = "env"
	SourceGit  Source = "git"
)

// RepoContext holds resolved repository information.
type RepoContext struct {
	Owner  string
	Name   string
	NWO    string // owner/name
	Source Source
}

var (
	sshRemoteRe   = regexp.MustCompile(`github\.com[:/]([^/]+)/([^/]+?)(?:\.git)?$`)
	httpsRemoteRe = regexp.MustCompile(`github\.com/([^/]+)/([^/]+?)(?:\.git)?$`)
)

// ResolveRepo resolves the target repository.
// Priority: flag value > GH_REPO env > git remote origin.
func ResolveRepo(flagValue string) *RepoContext {
	if flagValue != "" {
		return parseNWO(flagValue, SourceFlag)
	}
	if envRepo := os.Getenv("GH_REPO"); envRepo != "" {
		return parseNWO(envRepo, SourceEnv)
	}
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return nil
	}
	return parseRemoteURL(strings.TrimSpace(string(out)))
}

func parseNWO(nwo string, source Source) *RepoContext {
	parts := strings.Split(nwo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil
	}
	return &RepoContext{
		Owner:  parts[0],
		Name:   parts[1],
		NWO:    nwo,
		Source: source,
	}
}

func parseRemoteURL(url string) *RepoContext {
	if m := sshRemoteRe.FindStringSubmatch(url); len(m) == 3 {
		return &RepoContext{
			Owner:  m[1],
			Name:   m[2],
			NWO:    m[1] + "/" + m[2],
			Source: SourceGit,
		}
	}
	if m := httpsRemoteRe.FindStringSubmatch(url); len(m) == 3 {
		return &RepoContext{
			Owner:  m[1],
			Name:   m[2],
			NWO:    m[1] + "/" + m[2],
			Source: SourceGit,
		}
	}
	return nil
}
