package tui

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// A pull request the confirm popup skips is logged with why, as its row
// says it only for a while.
func TestSkippedIsLogged(t *testing.T) {
	f := fixture()
	f.info = map[model.PRKey]func(*model.MergeInfo){key("acme", "api", 30): func(mi *model.MergeInfo) { mi.State = model.StateClosed }}
	m := start(t, f, keymap.Default())
	var buf bytes.Buffer
	m.log = logging.New(&buf, slog.LevelInfo)
	// acme/api#30 was closed, so an approval leaves it out; acme/api#12 is
	// approved.
	selectAll(m, []model.PRKey{key("acme", "api", 12), key("acme", "api", 30)})
	press(m, "A")
	readPopup(t, m, m.renderBulk, "yes", "j")
	press(m, "y")
	log := buf.String()
	if !strings.Contains(log, `"msg":"pr skipped"`) || !strings.Contains(log, `"pr":"acme/api#30"`) || !strings.Contains(log, `"why":"closed"`) {
		t.Errorf("the skip is not logged:\n%s", log)
	}
	if strings.Contains(log, `"pr":"acme/api#12"`) {
		t.Errorf("one sent is logged as skipped:\n%s", log)
	}
}
