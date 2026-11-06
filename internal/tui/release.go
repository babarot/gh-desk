package tui

import (
	"context"
	"fmt"

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
	// Latest is the newest release a reading before this run found, and
	// Told the newest one already told of.
	Latest, Told string
	// UpdatedFrom is the version the last run was, when it was older than
	// Running; empty otherwise.
	UpdatedFrom string
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
	// they are.
	list []model.Release
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
	if r == nil {
		return nil
	}
	m.rel = releases{latest: r.Latest, told: r.Told}
	var cmds []tea.Cmd
	if r.UpdatedFrom != "" {
		m.log.Info("updated", logging.KeyOp, "release", logging.KeyVersion, r.Running, "from", r.UpdatedFrom)
		cmds = append(cmds, m.pushNote("✓", fmt.Sprintf("gh-desk updated to %s from %s", r.Running, r.UpdatedFrom), nil))
	}
	cmds = append(cmds, m.tellRelease())
	if r.Due {
		src := r.Source
		cmds = append(cmds, func() tea.Msg {
			list, err := src.Releases(m.ctx)
			return releasesMsg{list, err}
		})
	}
	return tea.Batch(cmds...)
}

// newerRelease is the newest release known that is newer than the one
// running, or empty.
func (m *Model) newerRelease() string {
	if r := m.set.Release; r != nil && release.Newer(m.rel.latest, r.Running) {
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
// nowhere but the log: the release is not the user's work.
func (m *Model) gotReleases(msg releasesMsg) tea.Cmd {
	latest := ""
	if msg.err != nil {
		m.log.Debug("releases not read", logging.KeyOp, "release", "err", msg.err.Error())
	} else {
		m.rel.list = msg.list
		for _, r := range msg.list {
			if latest == "" || release.Newer(r.Version, latest) {
				latest = r.Version
			}
		}
		m.log.Debug("releases read", logging.KeyOp, "release", "count", len(msg.list), "latest", latest)
		if latest != "" {
			m.rel.latest = latest
		}
	}
	if err := m.set.Release.Source.Checked(latest); err != nil {
		m.log.Warn("could not keep when the releases were read", logging.KeyOp, "release", "err", err.Error())
	}
	return m.tellRelease()
}
