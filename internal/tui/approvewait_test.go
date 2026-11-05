package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// An approval of a pull request whose jobs are at work waits for them,
// unless the user says to approve now.
func TestApprovalWaitsForChecks(t *testing.T) {
	f := fixture()
	k := key("acme", "api", 12)
	f.info = map[model.PRKey]func(*model.MergeInfo){k: func(mi *model.MergeInfo) {
		mi.Checks = []model.Check{{Name: "test", State: model.CheckPending, Kind: model.KindCheckRun, Run: "IN_PROGRESS"}}
	}}
	m := start(t, f, keymap.Default())
	selectAll(m, []model.PRKey{k})
	press(m, "A")
	text := readPopup(t, m, m.renderBulk, "yes", "j")
	has(t, text, "wait for checks → approve")
	has(t, text, "1 with checks still running are approved once they pass · w to approve now")
	press(m, "w")
	text = readPopup(t, m, m.renderBulk, "yes", "j")
	has(t, text, "1 with checks still running are approved now · w to wait for the checks")
	if it := m.bulk.items[0]; it.d.Step != "" || it.d.Reason != "approve" {
		t.Errorf("decision = %+v; want to approve now", it.d)
	}
}

// An approval written with R, with a text, waits for the jobs as one from
// A does, and w sends it at once.
func TestApprovalWithATextWaitsForChecks(t *testing.T) {
	f := fixture()
	k := key("acme", "api", 12)
	f.info = map[model.PRKey]func(*model.MergeInfo){k: func(mi *model.MergeInfo) {
		mi.Checks = []model.Check{{Name: "test", State: model.CheckPending, Kind: model.KindCheckRun, Run: "IN_PROGRESS"}}
	}}
	m, r := startWith(t, f, keymap.Default())
	press(m, "R", "tab")
	run(m, m.paste(tea.PasteMsg{Content: "Looks good once the tests pass."}))
	press(m, "ctrl+s")
	if m.bulk == nil || m.bulk.review.Event != model.EventApprove {
		t.Fatalf("the confirm popup of an approval did not open: %+v", m.bulk)
	}
	text := readPopup(t, m, m.renderBulk, "yes", "j")
	has(t, text, "wait for checks → approve")
	has(t, text, "1 with checks still running are approved once they pass · w to approve now")
	press(m, "y")
	if len(r.items) != 1 || len(r.items[0]) != 1 || !r.items[0][0].Wait {
		t.Errorf("items = %+v; want the approval to wait", r.items)
	}
}
