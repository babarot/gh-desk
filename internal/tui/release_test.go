package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

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
	m := startReleases(t, &Release{Running: "0.7.0", How: "gh extension upgrade desk", Latest: "0.7.0", Due: true, Source: f})
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
	m = startReleases(t, &Release{Running: "0.7.0", How: "gh extension upgrade desk", Latest: "0.8.0", Told: "0.8.0", Source: g})
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
	m := startReleases(t, &Release{Running: "0.7.0", How: "update it with Nix", Latest: "0.8.0", Told: "0.7.5", Source: f})
	if got := noteTitles(m); len(got) != 1 || got[0] != "↑ gh-desk 0.8.0 is out · update it with Nix" || f.read != 0 {
		t.Errorf("notes %q, read %d", got, f.read)
	}
}

func TestAnUpdateIsToldAtStart(t *testing.T) {
	f := &fakeReleases{}
	m := startReleases(t, &Release{Running: "0.8.0", How: "gh extension upgrade desk", Latest: "0.8.0", Told: "0.8.0", UpdatedFrom: "0.6.0", Source: f})
	if got := noteTitles(m); len(got) != 1 || got[0] != "✓ gh-desk updated to 0.8.0 from 0.6.0" {
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
			m := startReleases(t, &Release{Running: "0.7.0", How: "gh extension upgrade desk", Latest: "0.7.0", Due: true, Source: f})
			if len(m.notes) != 0 || len(f.told) != 0 || len(f.checked) != 1 {
				t.Errorf("notes %q, told %v, checked %v", noteTitles(m), f.told, f.checked)
			}
		})
	}
}

func TestHelpTellsOfANewerRelease(t *testing.T) {
	m := startReleases(t, &Release{Running: "0.7.0", How: "gh extension upgrade desk", Latest: "0.8.0", Told: "0.8.0", Source: &fakeReleases{}})
	m.set.Version = "0.7.0"
	press(m, "?")
	golden(t, "help-release", m)
}
