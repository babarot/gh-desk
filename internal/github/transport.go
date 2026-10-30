package github

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/babarot/gh-triage/internal/logging"
)

// transport logs every request to GitHub at debug, with GitHub's request ID
// and the time it took, and hands the request ID to the caller through the
// context, so a write can be logged at info with it.
type transport struct {
	next http.RoundTripper
	log  *slog.Logger
}

func (t transport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.next.RoundTrip(req)
	attrs := []any{"method", req.Method, "path", req.URL.Path, logging.KeyDurationMS, time.Since(start).Milliseconds()}
	if err != nil {
		t.log.Debug("github request failed", append(attrs, "err", err.Error())...)
		return resp, err
	}
	id := resp.Header.Get("X-GitHub-Request-Id")
	if p, ok := req.Context().Value(requestIDKey{}).(*string); ok {
		*p = id
	}
	t.log.Debug("github request", append(attrs, "status", resp.StatusCode, logging.KeyGHRequestID, id)...)
	return resp, nil
}

type requestIDKey struct{}

// withRequestID returns ctx carrying a place for the request ID of the last
// request made with it.
func withRequestID(ctx context.Context) (context.Context, *string) {
	id := new(string)
	return context.WithValue(ctx, requestIDKey{}, id), id
}
