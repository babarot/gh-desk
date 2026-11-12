package tui

import (
	"cmp"
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/release"
)

// Release is what gh-desk knows of its own releases at start, where it
// reads them, and where it keeps what it found.
type Release struct {
	// Running is the version running, and How says how this install is
	// updated.
	Running, How string
	// Tell tells of a newer release and of an update. Without it, the
	// releases are read only when their notes are opened.
	Tell bool
	// FromSource is set on a build that is not a release: it names the
	// last release as Running, but its code may be newer, so no release is
	// marked as the one installed.
	FromSource bool
	// Latest is the newest release a reading before this run found, and
	// Told the newest one already told of.
	Latest, Told string
	// UpdatedFrom is the version the last run was, when it was older than
	// Running; empty otherwise.
	UpdatedFrom string
	// NewSettings are the settings this version has that no version before
	// it did on this install, a table new as a whole by its name.
	NewSettings []string
	// Due reads the releases at start.
	Due    bool
	Source ReleaseSource
}

// ReleaseSource reads gh-desk's releases and keeps what was found for
// the next launch.
type ReleaseSource interface {
	// Releases reads the releases, newest first.
	Releases(ctx context.Context) ([]model.Release, error)
	// Checked keeps that the releases were read now and the newest found;
	// latest is empty when the reading failed, which keeps the newest
	// known.
	Checked(latest string) error
	// Told keeps the newest release told of, so it is told once.
	Told(version string) error
}

// releases is what this run knows of gh-desk's releases.
type releases struct {
	// latest is the newest release known, told the newest told of.
	latest, told string
	// list is the releases read in this run, newest first; nil until
	// they are. loading is set while they are read, and err is why the
	// last reading failed.
	list    []model.Release
	loading bool
	err     error
}

// releaseBox is the box of release notes, opened from the ? list; top is
// its first line shown.
type releaseBox struct {
	top int
}

type releasesMsg struct {
	list []model.Release
	err  error
}

// releaseStart tells of an update since the last run and of a newer
// release already known, and reads the releases when they are due. The
// reading never holds up the first frame.
func (m *Model) releaseStart() tea.Cmd {
	r := m.set.Release
	if r == nil || !r.Tell {
		return nil
	}
	m.rel.latest, m.rel.told = r.Latest, r.Told
	var cmds []tea.Cmd
	if r.UpdatedFrom != "" {
		m.log.Info("updated", logging.KeyOp, "release", logging.KeyVersion, r.Running, "from", r.UpdatedFrom)
		text := fmt.Sprintf("gh-desk updated to %s from %s", r.Running, r.UpdatedFrom)
		if ks := m.keys.Keys("release_notes"); len(ks) > 0 {
			text += fmt.Sprintf(" · %s %s what's new", m.firstKey("help"), ks[0])
		}
		cmds = append(cmds, m.pushNote("✓", text, nil))
	}
	if cmd := m.tellNewSettings(r.NewSettings); cmd != nil {
		cmds = append(cmds, cmd)
	}
	cmds = append(cmds, m.tellRelease())
	if r.Due {
		cmds = append(cmds, m.readReleases())
	}
	return tea.Batch(cmds...)
}

// tellNewSettings tells of the new settings as one notification, which
// lists them when they are more than one: a toast shows only the last one
// pushed.
func (m *Model) tellNewSettings(names []string) tea.Cmd {
	switch len(names) {
	case 0:
		return nil
	case 1:
		return m.pushNote("✓", fmt.Sprintf("new setting %s · gh desk config reference %s", names[0], names[0]), nil)
	}
	items := make([]noteItem, len(names))
	for i, n := range names {
		items[i] = noteItem{text: n}
	}
	return m.pushNote("✓", fmt.Sprintf("%d new settings · gh desk config reference", len(names)), items)
}

// readReleases reads the releases unless they are read or being read.
func (m *Model) readReleases() tea.Cmd {
	r := m.set.Release
	if r == nil || m.rel.list != nil || m.rel.loading {
		return nil
	}
	m.rel.loading, m.rel.err = true, nil
	src := r.Source
	return func() tea.Msg {
		list, err := src.Releases(m.ctx)
		return releasesMsg{list, err}
	}
}

// prefetchReleases reads the release notes as ? opens, when there is
// something new to read in them: a newer release, or an update. Otherwise
// they are read when opened, since ? is opened often to look up a key.
func (m *Model) prefetchReleases() tea.Cmd {
	if r := m.set.Release; r == nil || !r.Tell || m.newerRelease() == "" && r.UpdatedFrom == "" {
		return nil
	}
	return m.readReleases()
}

// newerRelease is the newest release known that is newer than the one
// running, or empty.
func (m *Model) newerRelease() string {
	if r := m.set.Release; r != nil && r.Tell && release.Newer(m.rel.latest, r.Running) {
		return m.rel.latest
	}
	return ""
}

// tellRelease tells, in a notification, of a newer release not told of
// before.
func (m *Model) tellRelease() tea.Cmd {
	v := m.newerRelease()
	if v == "" || v == m.rel.told {
		return nil
	}
	m.rel.told = v
	if err := m.set.Release.Source.Told(v); err != nil {
		m.log.Warn("could not keep the release told of", logging.KeyOp, "release", "err", err.Error())
	}
	return m.pushNote("↑", fmt.Sprintf("gh-desk %s is out · %s", v, m.set.Release.How), nil)
}

// gotReleases takes in a reading of the releases. A failed one is told of
// nowhere but the log and the box of release notes: the release is not
// the user's work.
func (m *Model) gotReleases(msg releasesMsg) tea.Cmd {
	m.rel.loading = false
	latest := ""
	if msg.err != nil {
		m.rel.err = msg.err
		m.log.Debug("releases not read", logging.KeyOp, "release", "err", msg.err.Error())
	} else {
		m.rel.list = msg.list
		if m.rel.list == nil {
			m.rel.list = []model.Release{}
		}
		for _, r := range msg.list {
			if latest == "" || release.Newer(r.Version, latest) {
				latest = r.Version
			}
		}
		m.log.Debug("releases read", logging.KeyOp, "release", "count", len(msg.list), "latest", latest)
	}
	if !m.set.Release.Tell {
		return nil
	}
	if latest != "" {
		m.rel.latest = latest
	}
	if err := m.set.Release.Source.Checked(latest); err != nil {
		m.log.Warn("could not keep when the releases were read", logging.KeyOp, "release", "err", err.Error())
	}
	return m.tellRelease()
}

// openReleaseNotes opens the box of release notes over the ? list, reading the
// releases when they are not read yet, or again after a failed reading.
func (m *Model) openReleaseNotes() tea.Cmd {
	if m.set.Release == nil {
		return nil
	}
	m.relBox = &releaseBox{}
	return m.readReleases()
}

// releaseSpec is the box of release notes as the shared popup layer reads
// it: a body of the notes. esc closes it back to the ? list, as it opened
// from it (ADR-125).
func (m *Model) releaseSpec() popupSpec {
	return popupSpec{
		closeWords: "back",
		// Its box takes no click: it is drawn where the ? list was.
		body:  &popupBody{scroll: m.scrollReleaseNotes, page: m.releaseNotesHeight(), click: func(int, int) {}},
		close: func() { m.relBox = nil },
	}
}

// scrollReleaseNotes moves the box of release notes by d lines, within its lines.
func (m *Model) scrollReleaseNotes(d int) {
	n := len(m.releaseNotesLines(m.releaseNotesWidth() - 4))
	m.relBox.top = max(min(m.relBox.top+d, n-m.releaseNotesHeight()), 0)
}

// releaseNotesWidth is the width of the box of release notes.
func (m *Model) releaseNotesWidth() int { return min(max(m.width-8, 30), 100) }

// releaseNotesHeight is how many lines of notes the box shows.
func (m *Model) releaseNotesHeight() int { return max(m.height-6, 3) }

// releaseNotesLines are the lines of the box of release notes, inner cells wide:
// every release read, newest first, with the one running marked installed
// and a newer one not installed with how to update, then its notes. A
// build from source marks none, and says what it is instead.
func (m *Model) releaseNotesLines(inner int) []string {
	st := m.st
	r := m.rel
	switch {
	case r.list == nil && r.err != nil:
		return []string{
			st.red.Render(fit("could not read the releases: "+r.err.Error(), inner)),
			"",
			st.muted.Render("they are at ") + st.blue.Render(release.Page),
		}
	case r.list == nil:
		return []string{m.working(st.muted, "reading the releases…")}
	case len(r.list) == 0:
		return []string{st.muted.Render("no releases yet")}
	}
	running := m.set.Release.Running
	var out []string
	if m.set.Release.FromSource {
		out = append(out, st.dim.Render(fit("this build: "+cmp.Or(m.set.Version, running)+", built from source", inner)), "")
	}
	for i, rel := range r.list {
		if i > 0 {
			out = append(out, "")
		}
		head := st.title.Render(rel.Version)
		if !rel.Published.IsZero() {
			head += "  " + st.dim.Render(rel.Published.Format("2006-01-02"))
		}
		switch c, err := release.Compare(rel.Version, running); {
		case err != nil || m.set.Release.FromSource:
		case c == 0:
			head += "  " + st.green.Render("installed")
		case c > 0:
			head += "  " + st.yellow.Render("not installed") + st.dim.Render(" · ") + st.key.Render(m.set.Release.How)
		}
		out = append(out, head)
		for _, l := range m.markdownLines(release.Notes(rel.Body), inner) {
			out = append(out, l.text)
		}
	}
	return out
}

// renderReleaseNotes draws the box of release notes over lines. It takes the
// keys while it is open, so its frame is in the focus color (ADR-079).
func (m *Model) renderReleaseNotes(lines []string) []string {
	st := m.st
	w := m.releaseNotesWidth()
	all := m.releaseNotesLines(w - 4)
	h := min(len(all), m.releaseNotesHeight())
	top := max(min(m.relBox.top, len(all)-h), 0)
	m.relBox.top = top
	title := strings.Join([]string{"release notes", m.scrollKeys() + " scroll", m.popupFootText(m.releaseSpec())}, " · ")
	b := box(st, title, all[top:top+h], w)
	if len(all) > h {
		b[len(b)-1] = scrollEdge(st, w, top, len(all)-top-h, m.scrollKeys()+" wheel")
	}
	return m.overlay(lines, b, max((len(lines)-len(b))/2, 0), m.width)
}
