package tui

import (
	"errors"

	"github.com/babarot/gh-desk/internal/model"
)

// errNotRead stands where a read would be, in a TUI that reads nothing
// (Settings.Offline): the settings page's preview, which shows the lists
// last read and must not reach GitHub.
var errNotRead = errors.New("not read in the preview")

// previewAssumes is what the merge confirm of a preview takes for true, as
// the lists last read do not say it.
const previewAssumes = "preview: assumes the repository allows every method and the pull request can merge as it is"

// errText draws an error of a read: dim for one that was not tried, red
// for one that failed.
func (m *Model) errText(err error) string {
	if errors.Is(err, errNotRead) {
		return m.st.dim.Render(err.Error())
	}
	return m.st.red.Render(err.Error())
}

// markNotRead says of the tabs the lists last read left empty that they
// were not read, so that none spins as if a read were coming.
func (m *Model) markNotRead() {
	for _, t := range m.tabs {
		if !t.loaded {
			t.loaded, t.err = true, errNotRead
			if t.inbox {
				t.inboxPRs = map[model.PRKey]model.PR{}
			}
		}
	}
}
