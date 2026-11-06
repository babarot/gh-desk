package github

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/logging"
)

const releasesBody = `[
  {"tag_name": "v0.9.0-rc.1", "prerelease": true, "published_at": "2026-11-08T00:00:00Z", "body": "rc", "html_url": "https://github.com/babarot/gh-desk/releases/tag/v0.9.0-rc.1"},
  {"tag_name": "v0.8.0", "published_at": "2026-11-07T00:00:00Z", "body": "## What's Changed", "html_url": "https://github.com/babarot/gh-desk/releases/tag/v0.8.0"},
  {"tag_name": "v0.7.1", "draft": true, "body": "draft"},
  {"tag_name": "v0.7.0", "published_at": "2026-11-05T00:00:00Z", "body": "notes", "html_url": "https://github.com/babarot/gh-desk/releases/tag/v0.7.0"}
]`

func TestReleasesLeaveOutDraftsAndPreReleases(t *testing.T) {
	f := &fake{rules: []rule{{func(r *http.Request, _ string) bool { return r.URL.Path == "/repos/babarot/gh-desk/releases" }, releasesBody}}}
	c := newClient(t, f, logging.Discard())
	rs, err := c.Releases(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 2 || rs[0].Version != "0.8.0" || rs[1].Version != "0.7.0" {
		t.Fatalf("releases = %+v", rs)
	}
	if !rs[0].Published.Equal(time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)) || rs[0].Body != "## What's Changed" ||
		rs[0].URL != "https://github.com/babarot/gh-desk/releases/tag/v0.8.0" {
		t.Errorf("release = %+v", rs[0])
	}
	if len(f.sent) != 1 || f.sent[0].method != http.MethodGet {
		t.Errorf("sent %+v", f.sent)
	}
}

// headers is a transport that keeps the requests and answers with body.
type headers struct {
	body string
	reqs []*http.Request
}

func (h *headers) RoundTrip(r *http.Request) (*http.Response, error) {
	h.reqs = append(h.reqs, r)
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(h.body)), Request: r}, nil
}

// Logged in to another host, the releases are read from github.com
// without that host's token.
func TestReleasesFromAnotherHostSendNoToken(t *testing.T) {
	h := &headers{body: releasesBody}
	c, err := New(logging.Discard(), Options{Host: "ghe.example.com", AuthToken: "ghe-token", Transport: h})
	if err != nil {
		t.Fatal(err)
	}
	rs, err := c.Releases(context.Background())
	if err != nil || len(rs) != 2 {
		t.Fatalf("Releases = %+v, %v", rs, err)
	}
	if len(h.reqs) != 1 {
		t.Fatalf("%d requests", len(h.reqs))
	}
	r := h.reqs[0]
	if r.URL.Host != "api.github.com" || r.URL.Path != "/repos/babarot/gh-desk/releases" || r.Header.Get("Authorization") != "" {
		t.Errorf("request to %s with Authorization %q", r.URL, r.Header.Get("Authorization"))
	}
}
