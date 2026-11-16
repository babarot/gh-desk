package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cli/go-gh/v2/pkg/api"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
)

// asyncAPIVersion is the REST API version that documents the asynchronous
// merge (ADR-129).
const asyncAPIVersion = "2026-03-10"

// asyncResult is GitHub's answer to an asynchronous merge and to a read of
// one.
type asyncResult struct {
	Status  string
	Details struct {
		UUID    string
		Message string
	}
}

func (r asyncResult) toModel() model.MergeResult {
	return model.MergeResult{Status: model.MergeStatus(r.Status), ID: r.Details.UUID, Message: r.Details.Message}
}

// MergeAsync asks GitHub to merge the pull request, or to put it in the
// merge queue, through the asynchronous merge API, only if its head is
// still head. It is the only way GitHub merges a stacked pull request, and
// with it every open one below it (ADR-129). A merge asked before and still
// pending is answered as pending with no ID.
func (c *Client) MergeAsync(ctx context.Context, key model.PRKey, head string, req model.MergeRequest) (model.MergeResult, error) {
	body := map[string]any{"sha": head, "merge_action": "direct_merge", "bypass_rules": req.Bypass}
	if req.Queue {
		body["merge_action"] = "merge_queue"
	} else {
		body["merge_method"] = methodREST(req.Method)
	}
	b, err := json.Marshal(body)
	if err != nil {
		return model.MergeResult{}, err
	}
	path := fmt.Sprintf("repos/%s/%s/pulls/%d/merge-async", url.PathEscape(key.Owner), url.PathEscape(key.Repo), key.Number)
	ctx, reqID := withRequestID(ctx)
	var resp asyncResult
	err = c.async.DoWithContext(ctx, http.MethodPut, path, bytes.NewReader(b), &resp)
	attrs := []any{logging.KeyPR, key.String(), logging.KeyStep, "merge-async", "head", head, "queue", req.Queue,
		"bypass", req.Bypass, logging.KeyGHRequestID, *reqID}
	var he *api.HTTPError
	switch {
	case errors.As(err, &he) && he.StatusCode == http.StatusConflict:
		c.log.Info("merge-async already pending", attrs...)
		return model.MergeResult{Status: model.MergePending}, nil
	case err != nil && ctx.Err() == nil && (!errors.As(err, &he) || he.StatusCode >= 500):
		// No answer, or GitHub failing: it may have taken the request.
		// Answered as one pending but unnamed, which is asked again.
		c.log.Info("merge-async not answered", append(attrs, "err", err.Error())...)
		return model.MergeResult{Status: model.MergePending}, nil
	case err != nil:
		c.log.Info("merge-async failed", append(attrs, "err", err.Error())...)
		return model.MergeResult{}, err
	}
	out := resp.toModel()
	c.log.Info("merge-async done", append(attrs, "status", string(out.Status))...)
	return out, nil
}

// MergeStatus reads where an asynchronous merge asked before stands; gone
// when GitHub no longer knows it.
func (c *Client) MergeStatus(ctx context.Context, key model.PRKey, id string) (model.MergeResult, error) {
	path := fmt.Sprintf("repos/%s/%s/pulls/%d/merge-async/%s", url.PathEscape(key.Owner), url.PathEscape(key.Repo), key.Number, url.PathEscape(id))
	var resp asyncResult
	if err := c.async.DoWithContext(ctx, http.MethodGet, path, nil, &resp); err != nil {
		var he *api.HTTPError
		if errors.As(err, &he) && he.StatusCode == http.StatusNotFound {
			return model.MergeResult{Status: model.MergeGone}, nil
		}
		return model.MergeResult{}, err
	}
	out := resp.toModel()
	c.log.Debug("merge-async read", logging.KeyPR, key.String(), "status", string(out.Status))
	return out, nil
}

// methodREST spells a merge method as REST does.
func methodREST(m model.MergeMethod) string {
	switch m {
	case model.MethodSquash:
		return "squash"
	case model.MethodRebase:
		return "rebase"
	}
	return "merge"
}
