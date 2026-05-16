package api

import "context"

// Branch is the compact ghub view of a GitHub branch.
type Branch struct {
	Name      string `json:"name"`
	SHA       string `json:"sha"`
	Protected bool   `json:"protected"`
}

type branchWire struct {
	Name      string               `json:"name"`
	Protected bool                 `json:"protected"`
	Commit    struct{ SHA string } `json:"commit"`
}

func (w branchWire) toBranch() Branch {
	return Branch{Name: w.Name, SHA: w.Commit.SHA, Protected: w.Protected}
}

// ListBranchesOpts controls ListBranches.
type ListBranchesOpts struct {
	Owner     string
	Repo      string
	Protected *bool // nil = all
	Limit     int
	Cursor    string
}

// ListBranches lists branches for a repository.
func (c *Client) ListBranches(ctx context.Context, opts ListBranchesOpts) ([]Branch, string, error) {
	if opts.Limit <= 0 {
		opts.Limit = 25
	}
	path := opts.Cursor
	if path == "" {
		qs := map[string]string{"per_page": "100"}
		if opts.Protected != nil {
			if *opts.Protected {
				qs["protected"] = "true"
			} else {
				qs["protected"] = "false"
			}
		}
		path = AppendQuery("/repos/"+opts.Owner+"/"+opts.Repo+"/branches", qs)
	}

	out := make([]Branch, 0, opts.Limit)
	next := path
	for next != "" && len(out) < opts.Limit {
		raw, n, err := c.Do(ctx, "GET", next, nil)
		if err != nil {
			return nil, "", err
		}
		var page []branchWire
		if err := DecodeInto(raw, &page); err != nil {
			return nil, "", err
		}
		for _, w := range page {
			if len(out) >= opts.Limit {
				break
			}
			out = append(out, w.toBranch())
		}
		next = n
	}
	if len(out) >= opts.Limit && next != "" {
		return out, next, nil
	}
	return out, "", nil
}
