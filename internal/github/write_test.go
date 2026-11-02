package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/babarot/gh-triage/internal/logging"
	"github.com/babarot/gh-triage/internal/model"
)

var (
	writeKey = model.PRKey{Owner: "acme", Repo: "tool", Number: 7}
	writePR  = model.PR{ID: "PR_1", Key: writeKey, HeadOID: "abc123"}
)

// secret is text that must never reach the log: a comment's body.
const secret = "a comment body that must stay out of the log"

// A write is one request, sent with what keeps it to the pull request
// read; whether it worked or not, it is logged at info with the pull
// request, the step, its result and GitHub's request ID, and never with
// the token or the text sent.
func TestWritesAreSentAndLogged(t *testing.T) {
	tests := []struct {
		name string
		// call makes the write.
		call func(c *Client) error
		// mutation is in the GraphQL request; empty for REST.
		mutation string
		// vars are the GraphQL variables sent.
		vars map[string]any
		// method and path are the REST request.
		method, path string
		// ok and failed are the log messages.
		ok, failed string
		step       string
	}{
		{
			name: "merge",
			call: func(c *Client) error {
				return c.Merge(context.Background(), writeKey, "PR_1", "abc123", model.MethodSquash)
			},
			mutation: "mergePullRequest",
			vars:     map[string]any{"id": "PR_1", "head": "abc123", "method": "SQUASH"},
			ok:       "merge done", failed: "merge failed", step: "merge",
		},
		{
			name: "enable auto-merge",
			call: func(c *Client) error {
				return c.EnableAutoMerge(context.Background(), writeKey, "PR_1", "abc123", model.MethodMerge)
			},
			mutation: "enablePullRequestAutoMerge",
			vars:     map[string]any{"id": "PR_1", "head": "abc123", "method": "MERGE"},
			ok:       "enable-auto-merge done", failed: "enable-auto-merge failed", step: "enable-auto-merge",
		},
		{
			name:     "ready for review",
			call:     func(c *Client) error { return c.MarkReady(context.Background(), writeKey, "PR_1", "abc123") },
			mutation: "markPullRequestReadyForReview",
			vars:     map[string]any{"id": "PR_1"},
			ok:       "ready-for-review done", failed: "ready-for-review failed", step: "ready-for-review",
		},
		{
			name:     "update branch",
			call:     func(c *Client) error { return c.UpdateBranch(context.Background(), writeKey, "PR_1", "abc123") },
			mutation: "updatePullRequestBranch",
			vars:     map[string]any{"id": "PR_1", "head": "abc123"},
			ok:       "update-branch done", failed: "update-branch failed", step: "update-branch",
		},
		{
			name:     "review",
			call:     func(c *Client) error { return c.Review(context.Background(), writePR, model.EventComment, secret) },
			mutation: "addPullRequestReview",
			vars:     map[string]any{"id": "PR_1", "event": "COMMENT", "body": secret, "commit": "abc123"},
			ok:       "review sent", failed: "review failed", step: "comment",
		},
		{
			name:     "comment",
			call:     func(c *Client) error { return c.PostComment(context.Background(), writePR, secret) },
			mutation: "addComment",
			vars:     map[string]any{"id": "PR_1", "body": secret},
			ok:       "comment posted", failed: "comment failed", step: "comment",
		},
		{
			name: "notification done",
			call: func(c *Client) error {
				return c.MarkDone(context.Background(), model.Notification{ThreadID: "123", Key: writeKey})
			},
			method: http.MethodDelete, path: "/notifications/threads/123",
			ok: "notification done", failed: "notification done failed", step: "notification-done",
		},
	}
	for _, tt := range tests {
		for _, fails := range []bool{false, true} {
			name := tt.name
			if fails {
				name += " failing"
			}
			t.Run(name, func(t *testing.T) {
				f := &fake{}
				switch {
				case tt.mutation != "" && fails:
					f.rules = []rule{{gqlHas(tt.mutation), `{"errors": [{"message": "Head branch was modified"}]}`}}
				case tt.mutation != "":
					f.rules = []rule{{gqlHas(tt.mutation), `{"data": {}}`}}
				case !fails:
					f.rules = []rule{{func(r *http.Request, _ string) bool { return r.Method == tt.method && r.URL.Path == tt.path }, ``}}
				}
				var logs bytes.Buffer
				err := tt.call(newClient(t, f, logging.New(&logs, slog.LevelInfo)))
				if (err != nil) != fails {
					t.Fatalf("err = %v", err)
				}

				if len(f.sent) != 1 {
					t.Fatalf("sent %d requests: %+v", len(f.sent), f.sent)
				}
				s := f.sent[0]
				if tt.mutation != "" {
					var req struct{ Variables map[string]any }
					if err := json.Unmarshal([]byte(s.body), &req); err != nil {
						t.Fatal(err)
					}
					if !equalVars(req.Variables, tt.vars) {
						t.Errorf("variables = %v, want %v", req.Variables, tt.vars)
					}
				} else if s.method != tt.method || s.path != tt.path {
					t.Errorf("sent %s %s", s.method, s.path)
				}

				line := logLine(t, logs.String())
				msg := tt.ok
				if fails {
					msg = tt.failed
				}
				if line["level"] != "INFO" || line["msg"] != msg || line[logging.KeyPR] != "acme/tool#7" || line[logging.KeyStep] != tt.step {
					t.Errorf("log = %v", line)
				}
				if _, ok := line[logging.KeyGHRequestID]; !ok {
					t.Errorf("log has no %s: %v", logging.KeyGHRequestID, line)
				}
				if !fails && line[logging.KeyGHRequestID] != "REQ-1" {
					t.Errorf("request ID = %v", line[logging.KeyGHRequestID])
				}
				if _, ok := line["err"]; ok != fails {
					t.Errorf("err in the log = %v, want %v", ok, fails)
				}
				for _, never := range []string{"test-token", secret} {
					if strings.Contains(logs.String(), never) {
						t.Errorf("%q was logged: %s", never, logs.String())
					}
				}
			})
		}
	}
}

// GitHub wants a pull request that can already be merged merged directly;
// enabling auto-merge on one says so with ErrCleanStatus.
func TestEnableAutoMergeOnACleanPullRequest(t *testing.T) {
	f := &fake{rules: []rule{{gqlHas("enablePullRequestAutoMerge"), `{"errors": [{"message": "Pull request is in clean status"}]}`}}}
	err := newClient(t, f, logging.Discard()).EnableAutoMerge(context.Background(), writeKey, "PR_1", "abc123", model.MethodMerge)
	if !errors.Is(err, ErrCleanStatus) {
		t.Errorf("err = %v, want ErrCleanStatus", err)
	}
}

// logLine is the one line logged at info, decoded.
func logLine(t *testing.T, logs string) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(logs), "\n")
	if len(lines) != 1 {
		t.Fatalf("logged %d lines at info: %s", len(lines), logs)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func equalVars(got, want map[string]any) bool {
	if len(got) != len(want) {
		return false
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}
