// Package api is a thin GitHub REST v3 client used by the ghub CLI.
//
// Design notes:
//   - Errors are mapped to typed exit codes (3=not_found, 4=auth, 5=api,
//     6=conflict, 7=rate_limit, 8=network, 9=validation) so callers can
//     return them straight from RunE.
//   - The client honors a context for cancellation; cmd layer is responsible
//     for setting a deadline if needed.
//   - Pagination is opaque: Do() returns the raw bytes plus the URL from the
//     Link header (rel="next"), which the cmd layer surfaces as --cursor.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/kdubb1337/ghub/internal/output"
)

// DefaultBaseURL is api.github.com. Override via GHUB_API_URL for GHES.
const DefaultBaseURL = "https://api.github.com"

// Client is a minimal GitHub REST client.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
	UA      string
}

// New constructs a client with sensible defaults.
func New(token string) *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		Token:   token,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		UA:      "ghub-cli",
	}
}

// Do executes a request against path (joined onto BaseURL or used directly if
// already absolute — pagination URLs from Link are absolute).
// body may be nil. The response body is fully read; nextURL is the rel="next"
// link if present.
func (c *Client) Do(ctx context.Context, method, path string, body any) (raw []byte, nextURL string, err error) {
	u := path
	if !strings.HasPrefix(path, "http") {
		u = c.BaseURL + path
	}

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, "", output.Errorf(1, "encode_failed", "marshal request body: %v", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u, reqBody)
	if err != nil {
		return nil, "", output.Errorf(2, "bad_request", "build request: %v", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", c.UA)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, "", output.Errorf(8, "network", "request to %s failed: %v", u, err)
	}
	defer resp.Body.Close()

	raw, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", output.Errorf(8, "network", "read response: %v", err)
	}

	if resp.StatusCode >= 400 {
		return nil, "", mapHTTPError(resp, raw, method, u)
	}

	nextURL = parseNextLink(resp.Header.Get("Link"))
	return raw, nextURL, nil
}

// DecodeInto unmarshals raw into v.
func DecodeInto(raw []byte, v any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return output.Errorf(1, "decode_failed", "unmarshal response: %v", err)
	}
	return nil
}

// errBody is the canonical GitHub error envelope.
type errBody struct {
	Message string `json:"message"`
	Errors  []struct {
		Resource string `json:"resource"`
		Code     string `json:"code"`
		Field    string `json:"field"`
		Message  string `json:"message"`
	} `json:"errors"`
	DocumentationURL string `json:"documentation_url"`
}

func mapHTTPError(resp *http.Response, raw []byte, method, u string) error {
	var b errBody
	_ = json.Unmarshal(raw, &b)
	msg := b.Message
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}

	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		// 403 may also be rate-limit; honor that signal.
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return output.ErrorfHint(7, "rate_limit",
				"reset at "+resp.Header.Get("X-RateLimit-Reset"),
				"github rate limit exhausted on %s %s", method, u)
		}
		return output.ErrorfHint(4, "auth",
			"run `ghub auth add <account>` or set GITHUB_TOKEN",
			"github rejected credentials (%d): %s", resp.StatusCode, msg)
	case http.StatusNotFound:
		return output.Errorf(3, "not_found", "github 404 on %s %s: %s", method, u, msg)
	case http.StatusConflict:
		return output.Errorf(6, "conflict", "github 409 on %s %s: %s", method, u, msg)
	case http.StatusUnprocessableEntity:
		details := summarizeValidation(b)
		return output.Errorf(9, "validation",
			"github rejected input on %s %s: %s%s", method, u, msg, details)
	case http.StatusTooManyRequests:
		return output.ErrorfHint(7, "rate_limit",
			"honor Retry-After: "+resp.Header.Get("Retry-After"),
			"github rate-limited %s %s: %s", method, u, msg)
	}
	if resp.StatusCode >= 500 {
		return output.Errorf(5, "api", "github %d on %s %s: %s", resp.StatusCode, method, u, msg)
	}
	return output.Errorf(1, "http_error", "github %d on %s %s: %s", resp.StatusCode, method, u, msg)
}

func summarizeValidation(b errBody) string {
	if len(b.Errors) == 0 {
		return ""
	}
	parts := make([]string, 0, len(b.Errors))
	for _, e := range b.Errors {
		seg := e.Field
		if e.Code != "" {
			seg += " (" + e.Code + ")"
		}
		if e.Message != "" {
			seg += ": " + e.Message
		}
		parts = append(parts, seg)
	}
	return " — " + strings.Join(parts, "; ")
}

var nextLinkRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

func parseNextLink(h string) string {
	if h == "" {
		return ""
	}
	m := nextLinkRe.FindStringSubmatch(h)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// AppendQuery returns path with the given key=value pairs added (ignoring empty values).
func AppendQuery(path string, kv map[string]string) string {
	q := url.Values{}
	for k, v := range kv {
		if v == "" {
			continue
		}
		q.Set(k, v)
	}
	if len(q) == 0 {
		return path
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + q.Encode()
}

// FormatTime formats a Go time.Time as RFC3339 for display.
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// Stringer keeps fmt deps small.
var _ = fmt.Sprintf
