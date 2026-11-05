package github

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/logging"
)

// Points left under lowRemaining are warned of once, again only after
// they went back over it; a query that failed before GitHub said is not
// taken for none left.
func TestLowPointsAreWarnedOnce(t *testing.T) {
	var buf bytes.Buffer
	c := newClient(t, &fake{}, logging.New(&buf, slog.LevelInfo))
	at := func(remaining int) rateLimit {
		return rateLimit{Cost: 1, Remaining: remaining, ResetAt: mustTime(t, "2026-11-05T10:00:00Z")}
	}
	c.noteRateLimit("search", rateLimit{})
	c.noteRateLimit("search", at(4000))
	c.noteRateLimit("search", at(400))
	c.noteRateLimit("search", at(399))
	c.noteRateLimit("search", at(4999))
	c.noteRateLimit("search", at(100))
	if n := strings.Count(buf.String(), `"msg":"github points running low"`); n != 2 {
		t.Errorf("warned %d times, want 2:\n%s", n, buf.String())
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return tm
}
