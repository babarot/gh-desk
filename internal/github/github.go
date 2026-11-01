// Package github is gh-triage's only way to GitHub: it searches pull
// requests, reads notifications, details and diffs, and sends reviews, with
// the token and host gh is logged in with. It returns model types.
package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/cli/go-gh/v2/pkg/browser"

	"github.com/babarot/gh-triage/internal/logging"
	"github.com/babarot/gh-triage/internal/model"
)

// Client talks to GitHub.
type Client struct {
	gql  *api.GraphQLClient
	rest *api.RESTClient
	diff *api.RESTClient
	log  *slog.Logger
}

// Options configure a Client. Transport is for tests; nil uses the network.
type Options struct {
	Host      string
	Transport http.RoundTripper
	// AuthToken is for tests; empty uses gh's login.
	AuthToken string
}

// DefaultHost is the host gh uses when none is given: GH_HOST, or the one
// gh is logged in to.
func DefaultHost() string {
	h, _ := auth.DefaultHost()
	return h
}

// New returns a client for the host gh is logged in to.
func New(log *slog.Logger, o Options) (*Client, error) {
	next := o.Transport
	if next == nil {
		next = http.DefaultTransport
	}
	// LogIgnoreEnv: GH_DEBUG=api would make go-gh print request and
	// response bodies (reviews, descriptions, comments) to stderr, over
	// the TUI. Requests are logged by transport instead, without bodies.
	opts := api.ClientOptions{Host: o.Host, AuthToken: o.AuthToken, Transport: transport{next: next, log: log},
		Timeout: 30 * time.Second, LogIgnoreEnv: true}
	gql, err := api.NewGraphQLClient(opts)
	if err != nil {
		return nil, err
	}
	rest, err := api.NewRESTClient(opts)
	if err != nil {
		return nil, err
	}
	opts.Headers = map[string]string{"Accept": "application/vnd.github.diff"}
	diff, err := api.NewRESTClient(opts)
	if err != nil {
		return nil, err
	}
	return &Client{gql: gql, rest: rest, diff: diff, log: log}, nil
}

// rateLimit is asked for in every query, to log what each costs.
type rateLimit struct {
	Cost      int
	Remaining int
}

func (c *Client) query(ctx context.Context, name, q string, vars map[string]any, resp any, rl *rateLimit) error {
	err := c.gql.DoWithContext(ctx, q, vars, resp)
	if rl != nil {
		c.log.Debug("graphql", "query", name, logging.KeyGQLCost, rl.Cost, logging.KeyGQLRemaining, rl.Remaining)
	}
	return err
}

// Viewer is the login of the user gh is logged in as.
func (c *Client) Viewer(ctx context.Context) (string, error) {
	var resp struct{ Viewer struct{ Login string } }
	if err := c.query(ctx, "viewer", `query { viewer { login } }`, nil, &resp, nil); err != nil {
		return "", err
	}
	return resp.Viewer.Login, nil
}

// The fields of a pull request come in two parts. GitHub works out the
// heavy ones (the review decision, the checks' rollup, the line counts)
// for each pull request, which makes a search of 50 take seconds longer, so
// a list's search reads only the light ones and the heavy ones are read
// after, by node ID (Heavy).
const lightFields = `
fragment light on PullRequest {
  id number title url state isDraft createdAt updatedAt
  headRefName headRefOid baseRefName
  author { login __typename }
  repository { nameWithOwner }
  reviewRequests(first: 10) { nodes { requestedReviewer { __typename ... on User { login } ... on Team { slug } } } }
  latestReviews(first: 10) { nodes { author { login } state } }
}`

// heavyFields read up to 100 labels, the most a page holds; a lower limit
// left labels out of the pane and the triage prompt, and costs no fewer
// points.
const heavyFields = `
fragment heavy on PullRequest {
  headRefOid additions deletions changedFiles reviewDecision
  labels(first: 100) { nodes { name } }
  comments { totalCount }
  commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }
}`

// prFields are every field, for reads of a few pull requests.
const prFields = `
fragment pr on PullRequest { ...light ...heavy }` + lightFields + heavyFields

type prNode struct {
	ID             string
	Number         int
	Title          string
	URL            string
	State          string
	IsDraft        bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	HeadRefName    string
	HeadRefOid     string
	BaseRefName    string
	Additions      int
	Deletions      int
	ChangedFiles   int
	ReviewDecision string
	Author         *struct {
		Login    string
		Typename string `json:"__typename"`
	}
	Repository     struct{ NameWithOwner string }
	Labels         struct{ Nodes []struct{ Name string } }
	Comments       struct{ TotalCount int }
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer *struct {
				Typename string `json:"__typename"`
				Login    string
				Slug     string
			}
		}
	}
	LatestReviews struct {
		Nodes []struct {
			Author *struct{ Login string }
			State  string
		}
	}
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct{ State string }
			}
		}
	}
}

// toModel makes the pull request; heavy says the heavy fields were read.
func (n prNode) toModel(heavy bool) (model.PR, bool) {
	if n.Number == 0 {
		return model.PR{}, false // not a pull request
	}
	owner, repo, _ := strings.Cut(n.Repository.NameWithOwner, "/")
	p := model.PR{
		ID: n.ID, Key: model.PRKey{Owner: owner, Repo: repo, Number: n.Number},
		Title: n.Title, URL: n.URL, State: model.PRState(n.State), Draft: n.IsDraft,
		Created: n.CreatedAt, Updated: n.UpdatedAt, HeadRef: n.HeadRefName, HeadOID: n.HeadRefOid, BaseRef: n.BaseRefName,
		Additions: n.Additions, Deletions: n.Deletions, ChangedFiles: n.ChangedFiles, Comments: n.Comments.TotalCount,
		Decision: model.ReviewDecision(n.ReviewDecision),
	}
	if n.Author != nil {
		p.Author, p.AuthorIsBot = n.Author.Login, n.Author.Typename == "Bot"
	} else {
		p.Author = "ghost"
	}
	for _, l := range n.Labels.Nodes {
		p.Labels = append(p.Labels, l.Name)
	}
	for _, r := range n.ReviewRequests.Nodes {
		switch rr := r.RequestedReviewer; {
		case rr == nil:
		case rr.Typename == "Team":
			p.Requested = append(p.Requested, model.Reviewer{Login: rr.Slug, Team: true})
		case rr.Login != "":
			p.Requested = append(p.Requested, model.Reviewer{Login: rr.Login})
		}
	}
	for _, r := range n.LatestReviews.Nodes {
		if r.Author != nil {
			p.Reviews = append(p.Reviews, model.Review{Author: r.Author.Login, State: model.ReviewState(r.State)})
		}
	}
	if len(n.Commits.Nodes) > 0 && n.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
		p.Checks = model.CheckState(n.Commits.Nodes[0].Commit.StatusCheckRollup.State)
	}
	if heavy {
		p.HeavyHead = n.HeadRefOid
	}
	return p, true
}

const searchQuery = `query($q: String!, $first: Int!, $after: String) {
  rateLimit { cost remaining }
  search(query: $q, type: ISSUE, first: $first, after: $after) {
    issueCount
    pageInfo { hasNextPage endCursor }
    nodes { ...light }
  }
}` + lightFields

// Result is a search: the pull requests read, and how many there are in
// all, which can be more. Warnings are the errors GitHub gave for the parts
// it could not read, such as an organization that wants the token
// authorized for SAML SSO; the rest is still in PRs.
type Result struct {
	PRs      []model.PR
	Total    int
	Warnings []string
}

// Page is one page of a search: its pull requests, how many match in all,
// and the cursor of the next page, empty after the last.
type Page struct {
	PRs      []model.PR
	Total    int
	Next     string
	Warnings []string
}

// SearchPage reads first pull requests matching query, a GitHub search,
// after the cursor after (empty for the first page). GitHub's search takes
// longer the more it returns, so a caller showing results reads a small
// first page and larger ones after it.
func (c *Client) SearchPage(ctx context.Context, query string, first int, after string) (Page, error) {
	vars := map[string]any{"q": query, "first": min(max(first, 1), 100), "after": nil}
	if after != "" {
		vars["after"] = after
	}
	var resp struct {
		RateLimit rateLimit
		Search    struct {
			IssueCount int
			PageInfo   struct {
				HasNextPage bool
				EndCursor   string
			}
			Nodes []prNode
		}
	}
	var out Page
	if err := c.query(ctx, "search", searchQuery, vars, &resp, &resp.RateLimit); err != nil {
		var gqlErr *api.GraphQLError
		if !errors.As(err, &gqlErr) {
			return out, err
		}
		for _, e := range gqlErr.Errors {
			out.Warnings = append(out.Warnings, e.Message)
		}
		c.log.Warn("search read in part", "query", query, "err", err.Error())
	}
	out.Total = resp.Search.IssueCount
	for _, n := range resp.Search.Nodes {
		if p, ok := n.toModel(false); ok {
			out.PRs = append(out.PRs, p)
		}
	}
	if resp.Search.PageInfo.HasNextPage {
		out.Next = resp.Search.PageInfo.EndCursor
	}
	return out, nil
}

const heavyQuery = `query($ids: [ID!]!) {
  rateLimit { cost remaining }
  nodes(ids: $ids) { ... on PullRequest { id ...heavy } }
}` + heavyFields

// Heavy reads the heavy fields of pull requests by node ID, each for the
// head it has now. A node GitHub cannot return is left out.
func (c *Client) Heavy(ctx context.Context, ids []string) (map[string]model.Heavy, error) {
	out := map[string]model.Heavy{}
	if len(ids) == 0 {
		return out, nil
	}
	var resp struct {
		RateLimit rateLimit
		Nodes     []*prNode
	}
	err := c.query(ctx, "heavy", heavyQuery, map[string]any{"ids": ids}, &resp, &resp.RateLimit)
	var gqlErr *api.GraphQLError
	if err != nil && !errors.As(err, &gqlErr) {
		return out, err
	}
	for _, n := range resp.Nodes {
		if n == nil || n.ID == "" {
			continue
		}
		p, _ := prNode{HeadRefOid: n.HeadRefOid, Additions: n.Additions, Deletions: n.Deletions, ChangedFiles: n.ChangedFiles,
			ReviewDecision: n.ReviewDecision, Labels: n.Labels, Comments: n.Comments, Commits: n.Commits, Number: 1}.toModel(true)
		out[n.ID] = p.Heavy()
	}
	return out, nil
}

// Search returns up to limit pull requests matching query, reading them in
// pages of 50.
func (c *Client) Search(ctx context.Context, query string, limit int) (Result, error) {
	var out Result
	after := ""
	for len(out.PRs) < limit {
		pg, err := c.SearchPage(ctx, query, min(limit-len(out.PRs), 50), after)
		if err != nil {
			return out, err
		}
		out.Total = pg.Total
		out.PRs = append(out.PRs, pg.PRs...)
		out.Warnings = append(out.Warnings, pg.Warnings...)
		if pg.Next == "" {
			break
		}
		after = pg.Next
	}
	return out, nil
}

// PullRequests reads the pull requests named by keys, in one query; a key
// that names no pull request (deleted, or not visible) is left out.
func (c *Client) PullRequests(ctx context.Context, keys []model.PRKey) (map[model.PRKey]model.PR, error) {
	out := map[model.PRKey]model.PR{}
	if len(keys) == 0 {
		return out, nil
	}
	var b strings.Builder
	b.WriteString("query {\n  rateLimit { cost remaining }\n")
	for i, k := range keys {
		fmt.Fprintf(&b, "  p%d: repository(owner: %q, name: %q) { pullRequest(number: %d) { ...pr } }\n", i, k.Owner, k.Repo, k.Number)
	}
	b.WriteString("}" + prFields)
	var resp map[string]jsonRaw
	err := c.query(ctx, "pullRequests", b.String(), nil, &resp, nil)
	for i := range keys {
		raw, ok := resp[fmt.Sprintf("p%d", i)]
		if !ok || raw == nil {
			continue
		}
		var repo struct{ PullRequest *prNode }
		if decode(raw, &repo) != nil || repo.PullRequest == nil {
			continue
		}
		if p, ok := repo.PullRequest.toModel(true); ok {
			out[p.Key] = p
		}
	}
	// A pull request that is gone fails its part of the query; the rest is
	// still read.
	var gqlErr *api.GraphQLError
	if err != nil && !errors.As(err, &gqlErr) {
		return out, err
	}
	return out, nil
}

// Notifications reads the unread notifications about pull requests, up to
// limit.
func (c *Client) Notifications(ctx context.Context, limit int) ([]model.Notification, error) {
	var out []model.Notification
	for page := 1; len(out) < limit; page++ {
		var resp []struct {
			ID         string
			Reason     string
			Unread     bool
			UpdatedAt  time.Time `json:"updated_at"`
			Repository struct {
				FullName string `json:"full_name"`
			}
			Subject struct {
				Title            string
				URL              string
				LatestCommentURL string `json:"latest_comment_url"`
				Type             string
			}
		}
		if err := c.rest.DoWithContext(ctx, http.MethodGet, fmt.Sprintf("notifications?per_page=50&page=%d", page), nil, &resp); err != nil {
			return out, err
		}
		for _, n := range resp {
			if n.Subject.Type != "PullRequest" {
				continue
			}
			key, ok := keyFromAPIURL(n.Subject.URL)
			if !ok {
				continue
			}
			latest := n.Subject.LatestCommentURL
			if latest == n.Subject.URL {
				latest = ""
			}
			out = append(out, model.Notification{ThreadID: n.ID, Key: key, Title: n.Subject.Title, Reason: n.Reason,
				Unread: n.Unread, Updated: n.UpdatedAt, LatestCommentURL: latest})
		}
		if len(resp) < 50 {
			break
		}
	}
	return out, nil
}

// keyFromAPIURL reads https://api.github.com/repos/owner/repo/pulls/123.
func keyFromAPIURL(s string) (model.PRKey, bool) {
	u, err := url.Parse(s)
	if err != nil {
		return model.PRKey{}, false
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	// An Enterprise Server URL has /api/v3 before /repos.
	for len(parts) > 0 && parts[0] != "repos" {
		parts = parts[1:]
	}
	if len(parts) != 5 || parts[3] != "pulls" {
		return model.PRKey{}, false
	}
	k, err := model.ParsePRKey(parts[1] + "/" + parts[2] + "#" + parts[4])
	return k, err == nil
}

// Comment reads the comment at its API URL, as a notification gives it.
func (c *Client) Comment(ctx context.Context, apiURL string) (model.Comment, error) {
	u, err := url.Parse(apiURL)
	if err != nil {
		return model.Comment{}, err
	}
	var resp struct {
		User      struct{ Login string }
		Body      string
		CreatedAt time.Time `json:"created_at"`
	}
	path := strings.TrimPrefix(u.Path, "/api/v3")
	if err := c.rest.DoWithContext(ctx, http.MethodGet, strings.TrimPrefix(path, "/"), nil, &resp); err != nil {
		return model.Comment{}, err
	}
	return model.Comment{Author: resp.User.Login, Body: resp.Body, Created: resp.CreatedAt}, nil
}

const detailQuery = `query($owner: String!, $name: String!, $number: Int!) {
  rateLimit { cost remaining }
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      body
      comments(last: 20) { nodes { author { login } body createdAt } }
      commits(last: 1) { nodes { commit { statusCheckRollup { contexts(first: 100) { nodes {
        __typename
        ... on CheckRun { name status conclusion }
        ... on StatusContext { context state }
      } } } } } }
    }
  }
}`

// Detail reads what only one opened pull request needs: its body, its
// checks and its latest comments.
func (c *Client) Detail(ctx context.Context, key model.PRKey) (model.Detail, error) {
	var resp struct {
		RateLimit  rateLimit
		Repository struct {
			PullRequest *struct {
				Body     string
				Comments struct {
					Nodes []struct {
						Author    *struct{ Login string }
						Body      string
						CreatedAt time.Time
					}
				}
				Commits struct {
					Nodes []struct {
						Commit struct {
							StatusCheckRollup *struct {
								Contexts struct {
									Nodes []struct {
										Typename   string `json:"__typename"`
										Name       string
										Status     string
										Conclusion string
										Context    string
										State      string
									}
								}
							}
						}
					}
				}
			}
		}
	}
	vars := map[string]any{"owner": key.Owner, "name": key.Repo, "number": key.Number}
	if err := c.query(ctx, "detail", detailQuery, vars, &resp, &resp.RateLimit); err != nil {
		return model.Detail{}, err
	}
	pr := resp.Repository.PullRequest
	if pr == nil {
		return model.Detail{}, fmt.Errorf("%s not found", key)
	}
	d := model.Detail{Body: pr.Body}
	for _, cm := range pr.Comments.Nodes {
		author := "ghost"
		if cm.Author != nil {
			author = cm.Author.Login
		}
		d.Comments = append(d.Comments, model.Comment{Author: author, Body: cm.Body, Created: cm.CreatedAt})
	}
	if len(pr.Commits.Nodes) > 0 && pr.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
		for _, n := range pr.Commits.Nodes[0].Commit.StatusCheckRollup.Contexts.Nodes {
			switch n.Typename {
			case "CheckRun":
				st := model.CheckState(n.Conclusion)
				if n.Status != "COMPLETED" {
					st = model.CheckPending
				}
				d.Checks = append(d.Checks, model.Check{Name: n.Name, State: st})
			case "StatusContext":
				d.Checks = append(d.Checks, model.Check{Name: n.Context, State: model.CheckState(n.State)})
			}
		}
	}
	return d, nil
}

// Diff reads a pull request's diff at the head it was read at
// (pr.HeadOID), as git diff writes it: the comparison of that commit with
// its merge base on the base branch, which is what GitHub shows as the
// pull request's diff. Asking by the pull request's number would give the
// diff of whatever the head is now, which may be a commit pushed after the
// list was read and so not the one a review is sent for.
func (c *Client) Diff(ctx context.Context, pr model.PR) ([]byte, error) {
	if pr.HeadOID == "" || pr.BaseRef == "" {
		return nil, fmt.Errorf("%s: no head or base to compare", pr.Key)
	}
	path := fmt.Sprintf("repos/%s/%s/compare/%s...%s", pr.Key.Owner, pr.Key.Repo, url.PathEscape(pr.BaseRef), pr.HeadOID)
	resp, err := c.diff.RequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

const reviewMutation = `mutation($id: ID!, $event: PullRequestReviewEvent!, $body: String, $commit: GitObjectID) {
  addPullRequestReview(input: {pullRequestId: $id, event: $event, body: $body, commitOID: $commit}) {
    pullRequestReview { state }
  }
}`

// Review sends a review of the pull request at the head it was read at
// (pr.HeadOID), so the review is recorded against the commit the user saw.
// GitHub does not refuse it when the head has moved since: the review is
// kept against that commit, and whether an approval still counts for the
// newer head depends on the repository dismissing stale approvals. A
// merge that must not take unseen commits has to pass expectedHeadOid
// itself.
func (c *Client) Review(ctx context.Context, pr model.PR, event model.ReviewEvent, body string) error {
	ctx, reqID := withRequestID(ctx)
	vars := map[string]any{"id": pr.ID, "event": string(event), "body": body, "commit": pr.HeadOID}
	if body == "" {
		vars["body"] = nil
	}
	var resp struct{}
	err := c.gql.DoWithContext(ctx, reviewMutation, vars, &resp)
	attrs := []any{logging.KeyPR, pr.Key.String(), logging.KeyStep, strings.ToLower(string(event)), "head", pr.HeadOID, logging.KeyGHRequestID, *reqID}
	if err != nil {
		c.log.Info("review failed", append(attrs, "err", err.Error())...)
		return err
	}
	c.log.Info("review sent", attrs...)
	return nil
}

// MarkDone marks a notification thread done, as the inbox's Done does.
func (c *Client) MarkDone(ctx context.Context, n model.Notification) error {
	ctx, reqID := withRequestID(ctx)
	err := c.rest.DoWithContext(ctx, http.MethodDelete, "notifications/threads/"+url.PathEscape(n.ThreadID), nil, nil)
	attrs := []any{logging.KeyPR, n.Key.String(), logging.KeyStep, "notification-done", logging.KeyGHRequestID, *reqID}
	if err != nil {
		c.log.Info("notification done failed", append(attrs, "err", err.Error())...)
		return err
	}
	c.log.Info("notification done", attrs...)
	return nil
}

// Browse opens url in the browser gh uses, writing nothing to the terminal
// the TUI draws on.
func (c *Client) Browse(url string) error {
	return browser.New("", io.Discard, io.Discard).Browse(url)
}
