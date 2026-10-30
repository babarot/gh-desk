package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-triage/internal/model"
)

// A list's search reads only the light fields of its pull requests; the
// heavy ones (the review decision, the checks, the line counts) are read
// after, ten pull requests at a time and a few at once, the shown tab first.
const (
	heavyBatch    = 10
	heavyInFlight = 5
)

// heavyJob is one read of heavy fields, for a tab's reading gen.
type heavyJob struct {
	tab, gen int
	ids      []string
}

type heavyMsg struct {
	job heavyJob
	got map[string]model.Heavy
	err error
}

// queueHeavy reads the heavy fields of prs for tab i's reading gen.
func (m *Model) queueHeavy(i, gen int, prs []model.PR) tea.Cmd {
	t := m.tabs[i]
	var ids []string
	for _, p := range prs {
		if p.ID != "" {
			ids = append(ids, p.ID)
		}
	}
	for len(ids) > 0 {
		n := min(heavyBatch, len(ids))
		m.heavyQueue = append(m.heavyQueue, heavyJob{tab: i, gen: gen, ids: ids[:n]})
		t.heavyLeft++
		ids = ids[n:]
	}
	return m.dispatchHeavy()
}

// dispatchHeavy sends queued reads up to heavyInFlight, those of the shown
// tab first, dropping those of a reading started again since.
func (m *Model) dispatchHeavy() tea.Cmd {
	var cmds []tea.Cmd
	for m.heavyBusy < heavyInFlight && len(m.heavyQueue) > 0 {
		at := 0
		for i, j := range m.heavyQueue {
			if j.tab == m.cur {
				at = i
				break
			}
		}
		j := m.heavyQueue[at]
		m.heavyQueue = append(m.heavyQueue[:at], m.heavyQueue[at+1:]...)
		if j.gen != m.tabs[j.tab].gen {
			continue
		}
		m.heavyBusy++
		cmds = append(cmds, func() tea.Msg {
			got, err := m.src.Heavy(m.ctx, j.ids)
			return heavyMsg{job: j, got: got, err: err}
		})
	}
	return tea.Batch(cmds...)
}

// heavyRead puts what was read in every row of the same pull request at the
// same head. A read of an older reading of its tab is dropped: a newer one
// may have put a newer head in those rows, and its own reads follow.
func (m *Model) heavyRead(msg heavyMsg) tea.Cmd {
	m.heavyBusy--
	cmds := []tea.Cmd{m.dispatchHeavy()}
	t := m.tabs[msg.job.tab]
	if msg.job.gen != t.gen {
		return tea.Batch(cmds...)
	}
	t.heavyLeft--
	if msg.err != nil {
		m.log.Warn("heavy fields", "tab", t.name, "err", msg.err.Error())
	}
	var filled []model.PR
	for id, h := range msg.got {
		h.Labels = safeLabels(h.Labels)
		for _, tt := range m.tabs {
			for i := range tt.prs {
				if p := &tt.prs[i]; p.ID == id && p.HeadOID == h.HeadOID {
					p.SetHeavy(h)
					filled = append(filled, *p)
				}
			}
		}
	}
	cmds = append(cmds, m.ensureChecks(filled))
	if m.tabDone(t) {
		cmds = append(cmds, m.saveTab(msg.job.tab))
	}
	return tea.Batch(cmds...)
}

// tabDone reports whether a tab's reading is over, heavy fields included.
func (m *Model) tabDone(t *tab) bool { return !t.loading && t.heavyLeft == 0 }

// reading reports whether anything is being read that a spinner shows:
// the lists, the pull requests of a confirm popup, or Claude's answers.
func (m *Model) reading() bool {
	if m.bulk != nil && m.bulk.loading {
		return true
	}
	for _, c := range m.chats {
		if c.running {
			return true
		}
	}
	for _, t := range m.tabs {
		if !m.tabDone(t) {
			return true
		}
	}
	return false
}

// spinMsg turns the spinner of what is being read.
type spinMsg struct{}

const spinEvery = 100 * time.Millisecond

var spinFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

// spinner is the spinner's frame now.
func (m *Model) spinner() string {
	return string(spinFrames[int(m.set.Now().UnixMilli()/spinEvery.Milliseconds())%len(spinFrames)])
}

// spin keeps the screen drawn while something is read, and only then.
func (m *Model) spin() tea.Cmd {
	if m.spinning || !m.reading() {
		return nil
	}
	m.spinning = true
	return tea.Tick(spinEvery, func(time.Time) tea.Msg { return spinMsg{} })
}

// freshMsg ends the mark on the rows a reading added or changed.
type freshMsg struct{ gen int }

const freshFor = 2 * time.Second
