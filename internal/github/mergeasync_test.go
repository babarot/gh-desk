package github

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"net/http"
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
