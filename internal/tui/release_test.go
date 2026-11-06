package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// fakeReleases is gh-desk's releases on a made-up GitHub, and what was kept
// of them.
type fakeReleases struct {
	list    []model.Release
	err     error
	read    int
	checked []string
	told    []string
}

func (f *fakeReleases) Releases(context.Context) ([]model.Release, error) {
	f.read++
	return f.list, f.err
}

func (f *fakeReleases) Checked(latest string) error {
	f.checked = append(f.checked, latest)
	return nil
}

func (f *fakeReleases) Told(v string) error {
	f.told = append(f.told, v)
	return nil
}

func startReleases(t *testing.T, r *Release) *Model {
	t.Helper()
	s := testSettings()
	s.Release = r
	m := New(fixture(), &fakeRunner{}, &fakeClaude{}, keymap.Default(), nil, s)
	m.Update(tea.WindowSizeMsg{Width: 130, Height: 34})
	run(m, m.Init())
	return m
}

func noteTitles(m *Model) []string {
	var ts []string
	for _, n := range m.notes {
		ts = append(ts, n.icon+" "+n.title)
	}
	return ts
}

func TestNoReleaseSettingsTellNothing(t *testing.T) {
	m := startReleases(t, nil)
	if len(m.notes) != 0 || m.newerRelease() != "" {
		t.Errorf("notes %v, newer %q", noteTitles(m), m.newerRelease())
	}
}

func TestANewerReleaseReadIsToldOnce(t *testing.T) {
	f := &fakeReleases{list: []model.Release{{Version: "0.7.0"}, {Version: "0.8.0"}, {Version: "0.6.0"}}}
	m := startReleases(t, &Release{Tell: true, Running: "0.7.0", How: "gh extension upgrade desk", Latest: "0.7.0", Due: true, Source: f})
	if f.read != 1 || strings.Join(f.checked, ",") != "0.8.0" || strings.Join(f.told, ",") != "0.8.0" {
		t.Fatalf("read %d, checked %v, told %v", f.read, f.checked, f.told)
	}
	if got := noteTitles(m); len(got) != 1 || got[0] != "↑ gh-desk 0.8.0 is out · gh extension upgrade desk" {
		t.Errorf("notes %q", got)
	}
	if len(m.rel.list) != 3 {
		t.Errorf("releases kept %d", len(m.rel.list))
	}

	// The next launch knows of 0.8.0 and that it was told: it says nothing
	// more, and ? still tells of it.
	g := &fakeReleases{}
	m = startReleases(t, &Release{Tell: true, Running: "0.7.0", How: "gh extension upgrade desk", Latest: "0.8.0", Told: "0.8.0", Source: g})
	if len(m.notes) != 0 || g.read != 0 || len(g.told) != 0 {
		t.Errorf("notes %q, read %d, told %v", noteTitles(m), g.read, g.told)
	}
	press(m, "?")
	if !strings.Contains(screen(m), "0.8.0 is out · gh extension upgrade desk") {
		t.Errorf("? does not tell of 0.8.0:\n%s", screen(m))
	}
}

// A release known from an earlier reading but not told, as when gh-desk
// quit before the note was kept, is told at start without reading.
func TestAKnownReleaseNotToldIsToldAtStart(t *testing.T) {
	f := &fakeReleases{}
	m := startReleases(t, &Release{Tell: true, Running: "0.7.0", How: "update it with Nix", Latest: "0.8.0", Told: "0.7.5", Source: f})
	if got := noteTitles(m); len(got) != 1 || got[0] != "↑ gh-desk 0.8.0 is out · update it with Nix" || f.read != 0 {
		t.Errorf("notes %q, read %d", got, f.read)
	}
}

func TestAnUpdateIsToldAtStart(t *testing.T) {
	f := &fakeReleases{}
	m := startReleases(t, &Release{Tell: true, Running: "0.8.0", How: "gh extension upgrade desk", Latest: "0.8.0", Told: "0.8.0", UpdatedFrom: "0.6.0", Source: f})
	if got := noteTitles(m); len(got) != 1 || got[0] != "✓ gh-desk updated to 0.8.0 from 0.6.0 · ? w what's new" {
		t.Errorf("notes %q", got)
	}
	press(m, "?")
	if strings.Contains(screen(m), "is out") {
		t.Error("? tells of a release while the newest runs")
	}
}

// A reading that fails keeps when it was tried and tells nothing; one that
// finds no newer release, or a tag of another form, tells nothing either.
func TestReadingsThatTellNothing(t *testing.T) {
	for name, f := range map[string]*fakeReleases{
		"failed":    {err: errors.New("offline")},
		"not newer": {list: []model.Release{{Version: "0.7.0"}, {Version: "0.6.0"}}},
		"older":     {list: []model.Release{{Version: "0.6.0"}}},
		"no tags":   {list: []model.Release{{Version: "nightly"}}},
	} {
		t.Run(name, func(t *testing.T) {
			m := startReleases(t, &Release{Tell: true, Running: "0.7.0", How: "gh extension upgrade desk", Latest: "0.7.0", Due: true, Source: f})
			if len(m.notes) != 0 || len(f.told) != 0 || len(f.checked) != 1 {
				t.Errorf("notes %q, told %v, checked %v", noteTitles(m), f.told, f.checked)
			}
		})
	}
}

func TestHelpTellsOfANewerRelease(t *testing.T) {
	m := startReleases(t, &Release{Tell: true, Running: "0.7.0", How: "gh extension upgrade desk", Latest: "0.8.0", Told: "0.8.0", Source: &fakeReleases{}})
	m.set.Version = "0.7.0"
	press(m, "?")
	golden(t, "help-release", m)
}

// releaseList is gh-desk's releases on the made-up GitHub, newest first.
func releaseList() []model.Release {
	return []model.Release{
		{Version: "0.8.0", Published: now.Add(-24 * time.Hour), Body: "<!-- generated -->\n\n## What's Changed\n### New Features\n" +
			"* Show the release notes from ? by @alice in https://github.com/acme/tool/pull/9\n\n**Full Changelog**: https://github.com/acme/tool/compare/v0.7.0...v0.8.0"},
		{Version: "0.7.0", Published: now.Add(-72 * time.Hour), Body: "## What's Changed\n### Bug fixes\n" +
			"* Keep the cursor on its row by @bob in https://github.com/acme/tool/pull/7"},
	}
}

// Without telling, nothing is read until the notes are opened: ? alone
// reads nothing, and w reads once.
func TestReleaseNotesWithoutTelling(t *testing.T) {
	f := &fakeReleases{list: releaseList()}
	m := startReleases(t, &Release{Running: "0.7.0", How: "gh extension upgrade desk", Latest: "0.8.0", Due: true, Source: f})
	if len(m.notes) != 0 || f.read != 0 {
		t.Fatalf("notes %q, read %d", noteTitles(m), f.read)
	}
	press(m, "?")
	if f.read != 0 || strings.Contains(screen(m), "is out") || !strings.Contains(screen(m), "w release notes") {
		t.Fatalf("read %d; ? =\n%s", f.read, screen(m))
	}
	press(m, "w")
	if f.read != 1 || len(f.checked) != 0 || len(f.told) != 0 || len(m.notes) != 0 {
		t.Errorf("read %d, checked %v, told %v, notes %q", f.read, f.checked, f.told, noteTitles(m))
	}
	golden(t, "release-notes", m)
	press(m, "esc", "w")
	if f.read != 1 {
		t.Errorf("read again: %d", f.read)
	}
	press(m, "esc")
	if !m.help || m.relBox != nil {
		t.Errorf("esc left help %v, box %v", m.help, m.relBox != nil)
	}
	press(m, "w", "?")
	if m.help || m.relBox != nil {
		t.Errorf("? left help %v, box %v", m.help, m.relBox != nil)
	}
}

// With a newer release known, ? reads the notes before w is pressed.
func TestReleaseNotesReadAsHelpOpens(t *testing.T) {
	f := &fakeReleases{list: releaseList()}
	m := startReleases(t, &Release{Tell: true, Running: "0.7.0", How: "gh extension upgrade desk", Latest: "0.8.0", Told: "0.8.0", Source: f})
	if f.read != 0 {
		t.Fatalf("read %d at start", f.read)
	}
	press(m, "?")
	if f.read != 1 || m.rel.list == nil {
		t.Fatalf("read %d as ? opened", f.read)
	}
	press(m, "w")
	if f.read != 1 || !strings.Contains(screen(m), "not installed · gh extension upgrade desk") || !strings.Contains(screen(m), "installed") {
		t.Errorf("read %d; box =\n%s", f.read, screen(m))
	}
}

// With nothing new, ? does not read; w does.
func TestReleaseNotesNotReadForNothingNew(t *testing.T) {
	f := &fakeReleases{list: releaseList()}
	m := startReleases(t, &Release{Tell: true, Running: "0.8.0", How: "gh extension upgrade desk", Latest: "0.8.0", Told: "0.8.0", Source: f})
	press(m, "?")
	if f.read != 0 {
		t.Fatalf("read %d as ? opened", f.read)
	}
	press(m, "w")
	if f.read != 1 {
		t.Errorf("read %d", f.read)
	}
}

// A failed reading says so in the box with where the releases are, and
// opening the box again reads again.
func TestReleaseNotesThatCannotBeRead(t *testing.T) {
	f := &fakeReleases{err: errors.New("dial tcp: lookup api.github.com: no such host")}
	m := startReleases(t, &Release{Running: "0.7.0", How: "gh extension upgrade desk", Source: f})
	press(m, "?", "w")
	if s := screen(m); !strings.Contains(s, "could not read the releases: dial tcp") || !strings.Contains(s, "https://github.com/babarot/gh-desk/releases") {
		t.Errorf("box =\n%s", s)
	}
	f.err, f.list = nil, releaseList()
	press(m, "esc", "w")
	if f.read != 2 || !strings.Contains(screen(m), "Show the release notes from ? (#9)") {
		t.Errorf("read %d; box =\n%s", f.read, screen(m))
	}
}

func TestReleaseNotesScroll(t *testing.T) {
	var list []model.Release
	for i := 20; i > 0; i-- {
		list = append(list, model.Release{Version: fmt.Sprintf("0.%d.0", i), Body: "### Bug fixes\n* Fix a thing (#1)"})
	}
	m := startReleases(t, &Release{Running: "0.20.0", How: "gh extension upgrade desk", Source: &fakeReleases{list: list}})
	press(m, "?", "w")
	if !strings.Contains(screen(m), "0.20.0") {
		t.Fatalf("box =\n%s", screen(m))
	}
	press(m, "G")
	if strings.Contains(screen(m), "0.20.0") || !strings.Contains(screen(m), "0.1.0") {
		t.Errorf("G did not go to the end:\n%s", screen(m))
	}
	press(m, "g")
	if !strings.Contains(screen(m), "0.20.0") {
		t.Errorf("g did not go to the top:\n%s", screen(m))
	}
}

// A build from source names the last release but may be newer: no release
// is marked installed, and the box says what the build is.
func TestReleaseNotesOfABuildFromSource(t *testing.T) {
	m := startReleases(t, &Release{FromSource: true, Running: "0.8.0", How: "https://github.com/babarot/gh-desk/releases",
		Source: &fakeReleases{list: releaseList()}})
	m.set.Version = "0.8.0 (e92582e)"
	press(m, "?", "w")
	s := screen(m)
	if !strings.Contains(s, "this build: 0.8.0 (e92582e), built from source") || strings.Contains(s, "installed") {
		t.Errorf("box =\n%s", s)
	}
}
