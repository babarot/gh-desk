package github

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"testing"

	"github.com/babarot/gh-desk/internal/model"
)

func restIs(method, path string) func(*http.Request, string) bool {
	return func(r *http.Request, _ string) bool { return r.Method == method && r.URL.Path == path }
}

// A merge goes to the asynchronous API with the confirmed head, in the API
// version that has it, and its request is read by its ID.
func TestMergeAsync(t *testing.T) {
	f := &fake{status: map[string]int{"/repos/acme/tool/pulls/7/merge-async": http.StatusAccepted}, rules: []rule{
		{match: restIs(http.MethodPut, "/repos/acme/tool/pulls/7/merge-async"),
			body: `{"status": "pending", "details": {"uuid": "u-1", "message": "Merge request accepted"}}`},
		{match: restIs(http.MethodGet, "/repos/acme/tool/pulls/7/merge-async/u-1"),
			body: `{"status": "failed", "details": {"message": "Required review missing"}}`},
	}}
	c := newClient(t, f, slog.New(slog.NewTextHandler(io.Discard, nil)))
	k := model.PRKey{Owner: "acme", Repo: "tool", Number: 7}
	got, err := c.MergeAsync(context.Background(), k, "abc123", model.MergeRequest{Method: model.MethodSquash, Bypass: true})
	if err != nil {
		t.Fatal(err)
	}
	if got != (model.MergeResult{Status: model.MergePending, ID: "u-1", Message: "Merge request accepted"}) {
		t.Errorf("result = %+v", got)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(f.sent[0].body), &body); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"sha": "abc123", "merge_method": "squash", "merge_action": "direct_merge", "bypass_rules": true}
	if !maps.Equal(body, want) || f.sent[0].version != asyncAPIVersion {
		t.Errorf("sent %v (version %q); want %v", body, f.sent[0].version, want)
	}
	st, err := c.MergeStatus(context.Background(), k, "u-1")
	if err != nil || st.Status != model.MergeFailed || st.Message != "Required review missing" {
		t.Errorf("status = %+v, %v", st, err)
	}
}

// Into the merge queue, no method is given: the queue merges by its own.
func TestMergeAsyncToTheQueue(t *testing.T) {
	f := &fake{rules: []rule{{match: restIs(http.MethodPut, "/repos/acme/tool/pulls/7/merge-async"),
		body: `{"status": "enqueued", "details": {"message": "Already in the merge queue"}}`}}}
	c := newClient(t, f, slog.New(slog.NewTextHandler(io.Discard, nil)))
	got, err := c.MergeAsync(context.Background(), model.PRKey{Owner: "acme", Repo: "tool", Number: 7}, "abc123", model.MergeRequest{Queue: true})
	if err != nil || got.Status != model.MergeEnqueued {
		t.Errorf("result = %+v, %v", got, err)
	}
	var body map[string]any
	_ = json.Unmarshal([]byte(f.sent[0].body), &body)
	if _, ok := body["merge_method"]; ok || body["merge_action"] != "merge_queue" {
		t.Errorf("sent %v", body)
	}
}

// A merge asked before and still pending is answered as pending, unnamed.
func TestMergeAsyncAlreadyPending(t *testing.T) {
	f := &fake{status: map[string]int{"/repos/acme/tool/pulls/7/merge-async": http.StatusConflict}, rules: []rule{{match: restIs(http.MethodPut, "/repos/acme/tool/pulls/7/merge-async"),
		body: `{"message": "A merge request is already enqueued for this pull request"}`}}}
	c := newClient(t, f, slog.New(slog.NewTextHandler(io.Discard, nil)))
	got, err := c.MergeAsync(context.Background(), model.PRKey{Owner: "acme", Repo: "tool", Number: 7}, "abc123", model.MergeRequest{})
	if err != nil || got != (model.MergeResult{Status: model.MergePending}) {
		t.Errorf("result = %+v, %v", got, err)
	}
}

// GitHub failing to answer may have taken the request: it is answered as
// pending and unnamed, to be asked again. A request GitHub no longer keeps
// is gone.
func TestMergeAsyncUnansweredAndGone(t *testing.T) {
	f := &fake{status: map[string]int{"/repos/acme/tool/pulls/7/merge-async": http.StatusBadGateway, "/repos/acme/tool/pulls/7/merge-async/u-1": http.StatusNotFound},
		rules: []rule{
			{match: restIs(http.MethodPut, "/repos/acme/tool/pulls/7/merge-async"), body: `{"message": "Bad Gateway"}`},
			{match: restIs(http.MethodGet, "/repos/acme/tool/pulls/7/merge-async/u-1"), body: `{"message": "Not Found"}`},
		}}
	c := newClient(t, f, slog.New(slog.NewTextHandler(io.Discard, nil)))
	k := model.PRKey{Owner: "acme", Repo: "tool", Number: 7}
	got, err := c.MergeAsync(context.Background(), k, "abc123", model.MergeRequest{})
	if err != nil || got != (model.MergeResult{Status: model.MergePending}) {
		t.Errorf("put = %+v, %v", got, err)
	}
	st, err := c.MergeStatus(context.Background(), k, "u-1")
	if err != nil || st.Status != model.MergeGone {
		t.Errorf("status = %+v, %v", st, err)
	}
}

const intoDefaultBody = `{"data": {
  "rateLimit": {"cost": 1, "remaining": 4999},
  "n0": {"id": "PR_1", "number": 7, "state": "OPEN", "headRefOid": "abc123", "baseRefName": "main",
    "baseRef": {"associatedPullRequests": {"nodes": [{"number": 6}]}},
    "repository": {"nameWithOwner": "acme/tool", "defaultBranchRef": {"name": "main"}},
    "latestReviews": {"nodes": []}, "commits": {"nodes": []}}
}}`

// A pull request into the default branch is stacked on none, though an
// open one, such as a release, has the default branch for its head.
func TestIntoTheDefaultBranchIsStackedOnNone(t *testing.T) {
	f := &fake{rules: []rule{{match: gqlHas("n0:"), body: intoDefaultBody}}}
	c := newClient(t, f, slog.New(slog.NewTextHandler(io.Discard, nil)))
	got, err := c.MergeInfo(context.Background(), []string{"PR_1"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if mi := got["PR_1"]; mi.BasePR != 0 || mi.Stacked {
		t.Errorf("base pull request = %d, stacked %v; want none", mi.BasePR, mi.Stacked)
	}
}

// Stacks are read on github.com only: another host's schema may not have
// them, and would refuse the whole read.
func TestStacksAreReadOnGitHubComOnly(t *testing.T) {
	anyHost := func(_ *http.Request, body string) bool { return strings.Contains(body, "n0:") }
	for host, want := range map[string]bool{"github.com": true, "ghe.example.com": false} {
		f := &fake{rules: []rule{{match: anyHost, body: intoDefaultBody}}}
		c, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Host: host, AuthToken: "test-token", Transport: f})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.MergeInfo(context.Background(), []string{"PR_1"}, false); err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(f.sent[0].body, "stack {"); got != want {
			t.Errorf("%s: stack read = %v", host, got)
		}
	}
}
