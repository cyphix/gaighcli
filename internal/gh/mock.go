package gh

import (
	"encoding/json"
	"sync"

	"github.com/cyphix/gaighcli/internal/context"
)

// MockRunner records calls and returns configured responses.
type MockRunner struct {
	mu       sync.Mutex
	Calls    [][]string
	Response map[string]MockResponse
}

type MockResponse struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

func (m *MockRunner) key(args []string) string {
	b, _ := json.Marshal(args)
	return string(b)
}

func (m *MockRunner) Run(args []string, ctx *context.RepoContext) (ExecResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Calls = append(m.Calls, append([]string{}, args...))
	key := m.key(args)
	if resp, ok := m.Response[key]; ok {
		if resp.Err != nil {
			return ExecResult{}, resp.Err
		}
		return ExecResult{Stdout: resp.Stdout, Stderr: resp.Stderr, ExitCode: resp.ExitCode}, nil
	}
	// Match without trailing --repo flags
	for k, resp := range m.Response {
		if stringsHasPrefixArgs(args, k) {
			if resp.Err != nil {
				return ExecResult{}, resp.Err
			}
			return ExecResult{Stdout: resp.Stdout, Stderr: resp.Stderr, ExitCode: resp.ExitCode}, nil
		}
	}
	return ExecResult{Stdout: "[]", ExitCode: 0}, nil
}

func stringsHasPrefixArgs(args []string, keyJSON string) bool {
	var keyArgs []string
	if err := json.Unmarshal([]byte(keyJSON), &keyArgs); err != nil {
		return false
	}
	if len(args) < len(keyArgs) {
		return false
	}
	for i, a := range keyArgs {
		if args[i] != a {
			return false
		}
	}
	return true
}

func (m *MockRunner) RunWithStdin(args []string, input string, ctx *context.RepoContext) (ExecResult, error) {
	return m.Run(args, ctx)
}
