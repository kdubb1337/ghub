package api

import (
	"context"
	"time"
)

// Repo is the compact ghub view of a GitHub repository.
// We deliberately keep only the high-gravity fields; pass --raw on the cmd
// layer if a richer dump is ever needed.
type Repo struct {
	ID            int64     `json:"id"`
	NodeID        string    `json:"node_id"`
	Name          string    `json:"name"`
	FullName      string    `json:"full_name"`
	Owner         string    `json:"owner"`
	Private       bool      `json:"private"`
	Visibility    string    `json:"visibility"`
	HTMLURL       string    `json:"html_url"`
	CloneURL      string    `json:"clone_url"`
	SSHURL        string    `json:"ssh_url"`
	DefaultBranch string    `json:"default_branch"`
	Description   string    `json:"description,omitempty"`
	Archived      bool      `json:"archived"`
	Fork          bool      `json:"fork"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	PushedAt      time.Time `json:"pushed_at,omitempty"`
}

// repoWire is the raw GitHub shape; we flatten owner.login.
type repoWire struct {
	ID            int64                  `json:"id"`
	NodeID        string                 `json:"node_id"`
	Name          string                 `json:"name"`
	FullName      string                 `json:"full_name"`
	Owner         struct{ Login string } `json:"owner"`
	Private       bool                   `json:"private"`
	Visibility    string                 `json:"visibility"`
	HTMLURL       string                 `json:"html_url"`
	CloneURL      string                 `json:"clone_url"`
	SSHURL        string                 `json:"ssh_url"`
	DefaultBranch string                 `json:"default_branch"`
	Description   string                 `json:"description"`
	Archived      bool                   `json:"archived"`
	Fork          bool                   `json:"fork"`
	CreatedAt     time.Time              `json:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at"`
	PushedAt      time.Time              `json:"pushed_at"`
}

func (w repoWire) toRepo() Repo {
	return Repo{
		ID: w.ID, NodeID: w.NodeID, Name: w.Name, FullName: w.FullName,
		Owner: w.Owner.Login, Private: w.Private, Visibility: w.Visibility,
		HTMLURL: w.HTMLURL, CloneURL: w.CloneURL, SSHURL: w.SSHURL,
		DefaultBranch: w.DefaultBranch, Description: w.Description,
		Archived: w.Archived, Fork: w.Fork,
		CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt, PushedAt: w.PushedAt,
	}
}

// CreateRepoOpts is the input for CreateRepo.
//
// When Org is empty the repo is created under the authenticated user via
// POST /user/repos. Otherwise POST /orgs/{Org}/repos.
type CreateRepoOpts struct {
	Name        string `json:"name"`
	Private     bool   `json:"private"`
	Description string `json:"description,omitempty"`
	AutoInit    bool   `json:"auto_init,omitempty"`
	Org         string `json:"-"`
}

// CreateRepo creates a new repository.
func (c *Client) CreateRepo(ctx context.Context, opts CreateRepoOpts) (Repo, error) {
	path := "/user/repos"
	if opts.Org != "" {
		path = "/orgs/" + opts.Org + "/repos"
	}
	raw, _, err := c.Do(ctx, "POST", path, opts)
	if err != nil {
		return Repo{}, err
	}
	var w repoWire
	if err := DecodeInto(raw, &w); err != nil {
		return Repo{}, err
	}
	return w.toRepo(), nil
}

// ListReposOpts controls ListRepos. Exactly one of User/Org may be set;
// when both are empty the authenticated user's repos are returned.
type ListReposOpts struct {
	User       string // GET /users/{user}/repos
	Org        string // GET /orgs/{org}/repos
	Visibility string // all | public | private (auth-user only)
	Limit      int    // max items to return across all pages
	Cursor     string // opaque pagination URL from previous call
}

// ListRepos returns up to opts.Limit repos and the next-cursor URL (empty if done).
func (c *Client) ListRepos(ctx context.Context, opts ListReposOpts) ([]Repo, string, error) {
	if opts.Limit <= 0 {
		opts.Limit = 25
	}
	path := opts.Cursor
	if path == "" {
		switch {
		case opts.Org != "":
			path = "/orgs/" + opts.Org + "/repos"
		case opts.User != "":
			path = "/users/" + opts.User + "/repos"
		default:
			path = "/user/repos"
		}
		qs := map[string]string{
			"per_page": "100",
			"sort":     "updated",
		}
		if opts.Visibility != "" && opts.Org == "" && opts.User == "" {
			qs["visibility"] = opts.Visibility
		}
		path = AppendQuery(path, qs)
	}

	out := make([]Repo, 0, opts.Limit)
	next := path
	for next != "" && len(out) < opts.Limit {
		raw, n, err := c.Do(ctx, "GET", next, nil)
		if err != nil {
			return nil, "", err
		}
		var page []repoWire
		if err := DecodeInto(raw, &page); err != nil {
			return nil, "", err
		}
		for _, w := range page {
			if len(out) >= opts.Limit {
				break
			}
			out = append(out, w.toRepo())
		}
		next = n
		if len(out) >= opts.Limit {
			break
		}
	}
	// If the inner loop hit limit mid-page, expose the next URL so callers can
	// resume; otherwise the loop drained naturally.
	if len(out) >= opts.Limit && next != "" {
		return out, next, nil
	}
	return out, "", nil
}
