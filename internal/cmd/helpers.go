package cmd

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/kdubb1337/ghub/internal/api"
	"github.com/kdubb1337/ghub/internal/auth"
	"github.com/kdubb1337/ghub/internal/output"
)

// newAPIClient resolves the active account, fetches its token, and returns a
// ready-to-use api.Client. The resolution honors:
//
//	GITHUB_TOKEN env > --account flag > GHUB_ACCOUNT env > saved default
//
// On any auth failure it returns a CLIError with exit code 4 so callers can
// `return err` directly.
func newAPIClient() (*api.Client, error) {
	account := flagAccount
	if account == "" && os.Getenv("GITHUB_TOKEN") == "" {
		if def, err := auth.DefaultAccount(); err == nil {
			account = def
		}
	}
	tok, err := auth.Token(account)
	if err != nil {
		return nil, output.ErrorfHint(4, "auth",
			"run `ghub auth add <account>` or set GITHUB_TOKEN",
			"no GitHub credentials available: %v", err)
	}
	c := api.New(tok)
	if base := os.Getenv("GHUB_API_URL"); base != "" {
		c.BaseURL = strings.TrimRight(base, "/")
	}
	return c, nil
}

// cmdContext returns a context with a 30-second deadline — matches the
// http.Client default and keeps `ghub` snappy for agents.
func cmdContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Second)
}

// parseRepoArg parses an "owner/repo" string. Returns a usage error (exit 2)
// on malformed input — the agent should fix the call, not retry.
func parseRepoArg(spec string) (owner, repo string, err error) {
	parts := strings.SplitN(spec, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", output.Errorf(2, "usage",
			"expected owner/repo, got %q", spec)
	}
	return parts[0], parts[1], nil
}

// resolveRepo returns owner/repo from the explicit --repo flag, or by parsing
// the current directory's `origin` remote when the flag is empty.
func resolveRepo(flagValue string) (owner, repo string, err error) {
	if flagValue != "" {
		return parseRepoArg(flagValue)
	}
	spec, derr := repoFromGitRemote()
	if derr != nil {
		return "", "", output.ErrorfHint(2, "usage",
			"pass --repo owner/name",
			"no --repo given and could not detect from git remote: %v", derr)
	}
	return parseRepoArg(spec)
}

// repoFromGitRemote runs `git config --get remote.origin.url` and parses the
// owner/repo from the result. Handles both ssh (git@github.com:o/r.git) and
// https (https://github.com/o/r.git) formats.
func repoFromGitRemote() (string, error) {
	out, err := exec.Command("git", "config", "--get", "remote.origin.url").Output()
	if err != nil {
		return "", err
	}
	url := strings.TrimSpace(string(out))
	url = strings.TrimSuffix(url, ".git")
	if strings.HasPrefix(url, "git@github.com:") {
		return strings.TrimPrefix(url, "git@github.com:"), nil
	}
	for _, p := range []string{"https://github.com/", "http://github.com/", "ssh://git@github.com/"} {
		if strings.HasPrefix(url, p) {
			return strings.TrimPrefix(url, p), nil
		}
	}
	return "", output.Errorf(2, "usage", "unrecognized remote URL: %s", url)
}

// currentBranch returns the active git branch, or "" on any error.
func currentBranch() string {
	out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
