package api

import (
	"context"
	"time"
)

// PR is the compact ghub view of a GitHub pull request.
type PR struct {
	Number    int        `json:"number"`
	Title     string     `json:"title"`
	State     string     `json:"state"`
	Draft     bool       `json:"draft"`
	Merged    bool       `json:"merged"`
	User      string     `json:"user"`
	HeadRef   string     `json:"head_ref"`
	BaseRef   string     `json:"base_ref"`
	HTMLURL   string     `json:"html_url"`
	Body      string     `json:"body,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ClosedAt  *time.Time `json:"closed_at,omitempty"`
	MergedAt  *time.Time `json:"merged_at,omitempty"`
}

type prWire struct {
	Number    int                    `json:"number"`
	Title     string                 `json:"title"`
	State     string                 `json:"state"`
	Draft     bool                   `json:"draft"`
	Merged    bool                   `json:"merged"`
	Body      string                 `json:"body"`
	HTMLURL   string                 `json:"html_url"`
	User      struct{ Login string } `json:"user"`
	Head      struct{ Ref string }   `json:"head"`
	Base      struct{ Ref string }   `json:"base"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	ClosedAt  *time.Time             `json:"closed_at"`
	MergedAt  *time.Time             `json:"merged_at"`
}

func (w prWire) toPR() PR {
	return PR{
		Number: w.Number, Title: w.Title, State: w.State,
		Draft: w.Draft, Merged: w.Merged, Body: w.Body,
		User: w.User.Login, HeadRef: w.Head.Ref, BaseRef: w.Base.Ref,
		HTMLURL:   w.HTMLURL,
		CreatedAt: w.CreatedAt, UpdatedAt: w.UpdatedAt,
		ClosedAt: w.ClosedAt, MergedAt: w.MergedAt,
	}
}

// ListPRsOpts controls ListPRs.
type ListPRsOpts struct {
	Owner  string
	Repo   string
	State  string // open | closed | all (default: open)
	Limit  int
	Cursor string
}

// ListPRs lists pull requests for a repo.
func (c *Client) ListPRs(ctx context.Context, opts ListPRsOpts) ([]PR, string, error) {
	if opts.Limit <= 0 {
		opts.Limit = 25
	}
	state := opts.State
	if state == "" {
		state = "open"
	}
	path := opts.Cursor
	if path == "" {
		path = AppendQuery("/repos/"+opts.Owner+"/"+opts.Repo+"/pulls",
			map[string]string{"state": state, "per_page": "100", "sort": "updated", "direction": "desc"})
	}

	out := make([]PR, 0, opts.Limit)
	next := path
	for next != "" && len(out) < opts.Limit {
		raw, n, err := c.Do(ctx, "GET", next, nil)
		if err != nil {
			return nil, "", err
		}
		var page []prWire
		if err := DecodeInto(raw, &page); err != nil {
			return nil, "", err
		}
		for _, w := range page {
			if len(out) >= opts.Limit {
				break
			}
			out = append(out, w.toPR())
		}
		next = n
	}
	if len(out) >= opts.Limit && next != "" {
		return out, next, nil
	}
	return out, "", nil
}

// GetPR fetches a single pull request.
func (c *Client) GetPR(ctx context.Context, owner, repo string, number int) (PR, error) {
	raw, _, err := c.Do(ctx, "GET", "/repos/"+owner+"/"+repo+"/pulls/"+itoa(number), nil)
	if err != nil {
		return PR{}, err
	}
	var w prWire
	if err := DecodeInto(raw, &w); err != nil {
		return PR{}, err
	}
	return w.toPR(), nil
}

// CreatePROpts is the input for CreatePR.
type CreatePROpts struct {
	Owner string `json:"-"`
	Repo  string `json:"-"`
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	Head  string `json:"head"`
	Base  string `json:"base"`
	Draft bool   `json:"draft,omitempty"`
}

// CreatePR opens a pull request.
func (c *Client) CreatePR(ctx context.Context, opts CreatePROpts) (PR, error) {
	raw, _, err := c.Do(ctx, "POST", "/repos/"+opts.Owner+"/"+opts.Repo+"/pulls", opts)
	if err != nil {
		return PR{}, err
	}
	var w prWire
	if err := DecodeInto(raw, &w); err != nil {
		return PR{}, err
	}
	return w.toPR(), nil
}

func itoa(n int) string {
	// Local tiny helper to keep this file self-contained (no strconv import).
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
