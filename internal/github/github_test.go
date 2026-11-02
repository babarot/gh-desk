package github

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/babarot/gh-triage/internal/logging"
	"github.com/babarot/gh-triage/internal/model"
)

// fake answers requests with canned bodies, by the first rule whose match
// returns true, and keeps what was sent. An empty body is answered 204 No
// Content, as GitHub answers a write it has nothing to say about.
type fake struct {
	mu    sync.Mutex
	rules []rule
	sent  []sent
}

type rule struct {
	match func(r *http.Request, body string) bool
	body  string
}

type sent struct {
	method, path, accept, body string
}

func (f *fake) RoundTrip(r *http.Request) (*http.Response, error) {
	var body string
	if r.Body != nil {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sent{r.Method, r.URL.Path, r.Header.Get("Accept"), body})
	for _, ru := range f.rules {
		if ru.match(r, body) {
			h := http.Header{"Content-Type": {"application/json"}, "X-Github-Request-Id": {"REQ-1"}}
			status := http.StatusOK
			if ru.body == "" {
				status = http.StatusNoContent
			}
			return &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(ru.body)), Request: r}, nil
		}
	}
	return &http.Response{StatusCode: 404, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"message":"Not Found"}`)), Request: r}, nil
}

func newClient(t *testing.T, f *fake, log *slog.Logger) *Client {
	t.Helper()
	c, err := New(log, Options{Host: "github.com", AuthToken: "test-token", Transport: f})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func gqlHas(s string) func(*http.Request, string) bool {
	return func(r *http.Request, body string) bool {
		return r.URL.Path == "/graphql" && strings.Contains(body, s)
	}
}

const searchBody = `{"data": {
  "rateLimit": {"cost": 2, "remaining": 4998},
  "search": {"issueCount": 3, "pageInfo": {"hasNextPage": false, "endCursor": ""}, "nodes": [
    {"id": "PR_1", "number": 7, "title": "Fix the parser", "url": "https://github.com/acme/tool/pull/7", "state": "OPEN",
     "isDraft": false, "createdAt": "2026-10-01T00:00:00Z", "updatedAt": "2026-10-02T00:00:00Z",
     "headRefName": "fix", "headRefOid": "abc123", "baseRefName": "main", "additions": 3, "deletions": 1, "changedFiles": 1,
     "reviewDecision": "REVIEW_REQUIRED", "author": {"login": "alice", "__typename": "User"},
     "repository": {"nameWithOwner": "acme/tool"}, "labels": {"nodes": [{"name": "bug"}]}, "comments": {"totalCount": 2},
     "reviewRequests": {"nodes": [{"requestedReviewer": {"__typename": "User", "login": "me"}},
                                  {"requestedReviewer": {"__typename": "Team", "slug": "core"}}]},
     "latestReviews": {"nodes": [{"author": {"login": "bob"}, "state": "APPROVED"}]},
     "commits": {"nodes": [{"commit": {"statusCheckRollup": {"state": "SUCCESS"}}}]}},
    {"id": "PR_2", "number": 8, "title": "Update dep", "url": "u", "state": "OPEN", "isDraft": true,
     "createdAt": "2026-10-01T00:00:00Z", "updatedAt": "2026-10-01T00:00:00Z",
     "author": {"login": "renovate", "__typename": "Bot"}, "repository": {"nameWithOwner": "acme/tool"},
     "labels": {"nodes": []}, "comments": {"totalCount": 0}, "reviewRequests": {"nodes": []},
     "latestReviews": {"nodes": []}, "commits": {"nodes": [{"commit": {"statusCheckRollup": null}}]}},
    {}
  ]}}}`

func TestSearch(t *testing.T) {
	f := &fake{rules: []rule{{gqlHas("search(query"), searchBody}}}
	res, err := newClient(t, f, logging.Discard()).Search(context.Background(), "is:pr", 50)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 3 || len(res.PRs) != 2 {
		t.Fatalf("total %d, prs %d", res.Total, len(res.PRs))
	}
	p := res.PRs[0]
	if p.Key.String() != "acme/tool#7" || p.HeadOID != "abc123" || p.Checks != model.CheckSuccess || p.AuthorIsBot {
		t.Errorf("pr = %+v", p)
	}
	if !p.RequestedFrom("me") || p.RequestedFrom("core") || len(p.Requested) != 2 || !p.Requested[1].Team {
		t.Errorf("requested = %+v", p.Requested)
	}
	if r, ok := p.ReviewBy("bob"); !ok || r.State != model.ReviewApproved {
		t.Errorf("reviews = %+v", p.Reviews)
	}
	if b := res.PRs[1]; !b.AuthorIsBot || !b.Draft || b.Checks != "" {
		t.Errorf("bot pr = %+v", b)
	}
}

func TestSearchKeepsPartialResults(t *testing.T) {
	body := strings.Replace(searchBody, `{"data":`, `{"errors": [{"message": "Resource protected by organization SAML enforcement"}], "data":`, 1)
	f := &fake{rules: []rule{{gqlHas("search(query"), body}}}
	res, err := newClient(t, f, logging.Discard()).Search(context.Background(), "is:pr", 50)
	if err != nil || len(res.PRs) != 2 || len(res.Warnings) != 1 {
		t.Errorf("res = %+v, err = %v", res, err)
	}
}

func TestReviewSendsTheHeadItWasReadAt(t *testing.T) {
	f := &fake{rules: []rule{{gqlHas("addPullRequestReview"), `{"data": {"addPullRequestReview": {"pullRequestReview": {"state": "APPROVED"}}}}`}}}
	var logs bytes.Buffer
	c := newClient(t, f, logging.New(&logs, slog.LevelInfo))
	pr := model.PR{ID: "PR_1", Key: model.PRKey{Owner: "acme", Repo: "tool", Number: 7}, HeadOID: "abc123"}
	if err := c.Review(context.Background(), pr, model.EventApprove, ""); err != nil {
		t.Fatal(err)
	}
	var req struct{ Variables map[string]any }
	if err := json.Unmarshal([]byte(f.sent[len(f.sent)-1].body), &req); err != nil {
		t.Fatal(err)
	}
	if req.Variables["commit"] != "abc123" || req.Variables["event"] != "APPROVE" || req.Variables["body"] != nil {
		t.Errorf("variables = %v", req.Variables)
	}
	for _, want := range []string{`"msg":"review sent"`, `"pr":"acme/tool#7"`, `"gh_request_id":"REQ-1"`} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("log lacks %s: %s", want, logs.String())
		}
	}
	if strings.Contains(logs.String(), "test-token") {
		t.Error("the token was logged")
	}
}

// The diff is pinned to the head the list read, not whatever the head is
// now.
func TestDiffAsksForTheDiffFormatAtTheHead(t *testing.T) {
	f := &fake{rules: []rule{{func(r *http.Request, _ string) bool { return r.URL.Path == "/repos/acme/tool/compare/main...abc123" }, "diff --git a/x b/x\n"}}}
	pr := model.PR{Key: model.PRKey{Owner: "acme", Repo: "tool", Number: 7}, HeadOID: "abc123", BaseRef: "main"}
	b, err := newClient(t, f, logging.Discard()).Diff(context.Background(), pr)
	if err != nil || !strings.HasPrefix(string(b), "diff --git") {
		t.Fatalf("diff = %q, %v", b, err)
	}
	if got := f.sent[0].accept; got != "application/vnd.github.diff" {
		t.Errorf("Accept = %q", got)
	}
}

func TestNotifications(t *testing.T) {
	body := `[
	  {"id": "1", "reason": "review_requested", "unread": true, "updated_at": "2026-10-01T00:00:00Z",
	   "repository": {"full_name": "acme/tool"},
	   "subject": {"title": "Fix", "url": "https://api.github.com/repos/acme/tool/pulls/7",
	               "latest_comment_url": "https://api.github.com/repos/acme/tool/pulls/7", "type": "PullRequest"}},
	  {"id": "2", "reason": "mention", "unread": true, "updated_at": "2026-10-01T00:00:00Z",
	   "repository": {"full_name": "acme/tool"},
	   "subject": {"title": "Ask", "url": "https://api.github.com/repos/acme/tool/pulls/8",
	               "latest_comment_url": "https://api.github.com/repos/acme/tool/issues/comments/99", "type": "PullRequest"}},
	  {"id": "3", "reason": "subscribed", "unread": true, "updated_at": "2026-10-01T00:00:00Z",
	   "repository": {"full_name": "acme/tool"},
	   "subject": {"title": "An issue", "url": "https://api.github.com/repos/acme/tool/issues/9", "type": "Issue"}}
	]`
	f := &fake{rules: []rule{{func(r *http.Request, _ string) bool { return r.URL.Path == "/notifications" }, body}}}
	ns, err := newClient(t, f, logging.Discard()).Notifications(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(ns) != 2 || ns[0].Key.String() != "acme/tool#7" || ns[0].LatestCommentURL != "" ||
		ns[1].LatestCommentURL == "" || ns[1].Reason != "mention" {
		t.Errorf("notifications = %+v", ns)
	}
}

func TestKeyFromAPIURL(t *testing.T) {
	for in, want := range map[string]string{
		"https://api.github.com/repos/acme/tool/pulls/7":         "acme/tool#7",
		"https://ghe.example.com/api/v3/repos/acme/tool/pulls/7": "acme/tool#7",
		"https://api.github.com/repos/acme/tool/issues/7":        "",
	} {
		k, ok := keyFromAPIURL(in)
		if (want == "") == ok || (ok && k.String() != want) {
			t.Errorf("keyFromAPIURL(%q) = %v, %v", in, k, ok)
		}
	}
}

func TestPullRequestsSkipsMissing(t *testing.T) {
	body := `{"errors": [{"message": "Could not resolve to a PullRequest"}], "data": {
	  "p0": {"pullRequest": {"id": "PR_1", "number": 7, "title": "t", "url": "u", "state": "MERGED",
	         "author": {"login": "a", "__typename": "User"}, "repository": {"nameWithOwner": "acme/tool"},
	         "labels": {"nodes": []}, "comments": {"totalCount": 0}, "reviewRequests": {"nodes": []},
	         "latestReviews": {"nodes": []}, "commits": {"nodes": []}}},
	  "p1": {"pullRequest": null}}}`
	f := &fake{rules: []rule{{gqlHas("p0: repository"), body}}}
	keys := []model.PRKey{{Owner: "acme", Repo: "tool", Number: 7}, {Owner: "acme", Repo: "tool", Number: 99}}
	got, err := newClient(t, f, logging.Discard()).PullRequests(context.Background(), keys)
	if err != nil || len(got) != 1 || got[keys[0]].State != model.StateMerged {
		t.Errorf("got %+v, %v", got, err)
	}
}

func TestPullRequestsSplitsQueries(t *testing.T) {
	f := &fake{rules: []rule{{gqlHas("p0: repository"), `{"data": {}}`}}}
	keys := make([]model.PRKey, 2*prsPerQuery+1)
	for i := range keys {
		keys[i] = model.PRKey{Owner: "acme", Repo: "tool", Number: i + 1}
	}
	if _, err := newClient(t, f, logging.Discard()).PullRequests(context.Background(), keys); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 3 {
		t.Fatalf("sent %d queries, want 3", len(f.sent))
	}
	for _, s := range f.sent {
		if n := strings.Count(s.body, "repository(owner:"); n > prsPerQuery {
			t.Errorf("a query read %d pull requests, more than %d", n, prsPerQuery)
		}
	}
}
