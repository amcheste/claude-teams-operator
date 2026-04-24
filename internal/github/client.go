// Package github is a minimal GitHub REST API client covering just what the
// operator needs for OnComplete=create-pr. Kept intentionally narrow — no
// external SDK, no webhooks, no GraphQL — so the dependency surface stays
// small and the blast radius of a bad token is easy to reason about.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the public GitHub API root. Overridable via WithBaseURL
// for GitHub Enterprise and for tests that stand up an httptest server.
const DefaultBaseURL = "https://api.github.com"

// DefaultTimeout bounds a single HTTP call. Short enough that a flaky API
// cannot stall the reconciler, long enough that a slow-but-healthy network
// round-trip still completes.
const DefaultTimeout = 15 * time.Second

// Client issues authenticated HTTP requests to the GitHub REST API.
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// Option customizes a Client at construction time.
type Option func(*Client)

// WithBaseURL overrides the API base URL. Used for GitHub Enterprise and
// httptest servers in tests.
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = strings.TrimSuffix(url, "/") }
}

// WithHTTPClient injects a custom http.Client (primarily for tests).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// NewClient returns a Client authenticated with the given token. An empty
// token is accepted but every request will fail at the API — callers should
// validate upstream.
func NewClient(token string, opts ...Option) *Client {
	c := &Client{
		baseURL: DefaultBaseURL,
		token:   token,
		http:    &http.Client{Timeout: DefaultTimeout},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// PullRequestRequest is the subset of the GitHub "create pull request" body
// the operator fills in. Optional reviewers/labels are applied via follow-up
// calls after PR creation.
type PullRequestRequest struct {
	Title string `json:"title"`
	Body  string `json:"body,omitempty"`
	Head  string `json:"head"`
	Base  string `json:"base"`
	Draft bool   `json:"draft,omitempty"`
}

// PullRequest is the subset of the GitHub pull request payload the operator
// persists into AgentTeam.status.pullRequest.
type PullRequest struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	State   string `json:"state"`
}

// CreatePullRequest opens a PR against owner/repo and returns the created
// resource. API errors are surfaced with the GitHub response body so
// "Validation Failed" messages propagate to kubectl describe.
func (c *Client) CreatePullRequest(ctx context.Context, owner, repo string, req *PullRequestRequest) (*PullRequest, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal PR request: %w", err)
	}
	path := fmt.Sprintf("/repos/%s/%s/pulls", url.PathEscape(owner), url.PathEscape(repo))
	var pr PullRequest
	if err := c.do(ctx, http.MethodPost, path, body, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// RequestReviewers adds reviewer usernames to an existing PR. No-op when
// the list is empty.
func (c *Client) RequestReviewers(ctx context.Context, owner, repo string, number int, reviewers []string) error {
	if len(reviewers) == 0 {
		return nil
	}
	body, err := json.Marshal(map[string][]string{"reviewers": reviewers})
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/repos/%s/%s/pulls/%d/requested_reviewers",
		url.PathEscape(owner), url.PathEscape(repo), number)
	return c.do(ctx, http.MethodPost, path, body, nil)
}

// AddLabels applies issue labels to the PR (PRs are issues under GitHub's
// data model). No-op when the list is empty.
func (c *Client) AddLabels(ctx context.Context, owner, repo string, number int, labels []string) error {
	if len(labels) == 0 {
		return nil
	}
	body, err := json.Marshal(map[string][]string{"labels": labels})
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/repos/%s/%s/issues/%d/labels",
		url.PathEscape(owner), url.PathEscape(repo), number)
	return c.do(ctx, http.MethodPost, path, body, nil)
}

// do performs an authenticated request and, when result is non-nil, decodes
// the JSON response into it. Non-2xx responses are returned as errors with
// the response body attached so upstream callers can record the failure on
// the AgentTeam status.
func (c *Client) do(ctx context.Context, method, path string, body []byte, result interface{}) error {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("GitHub API %s %s returned %d: %s",
			method, path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if result != nil && len(respBody) > 0 {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decode GitHub response: %w", err)
		}
	}
	return nil
}
