package tui

import (
	"maps"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/width"
)

// TestSpinnerFramesTakeOneCell keeps every frame one cell wide on any
// terminal: a frame in a column that took two would move the columns
// after it.
func TestSpinnerFramesTakeOneCell(t *testing.T) {
	all := maps.Clone(spinners)
	all["claude's own"] = claudeSpinner
	for name, s := range all {
		if len(s.frames) == 0 || s.every <= 0 {
			t.Errorf("%s has no frames or no interval", name)
		}
		for _, f := range s.frames {
			if w := lipgloss.Width(f); w != 1 {
				t.Errorf("%s: frame %q is %d cells wide", name, f, w)
			}
			for _, r := range f {
				switch width.LookupRune(r).Kind() {
				case width.EastAsianAmbiguous, width.EastAsianWide, width.EastAsianFullwidth:
					t.Errorf("%s: frame %q is two cells wide where East Asian Ambiguous characters are", name, f)
				}
			}
		}
	}
}

// at is m with the clock stopped at ms milliseconds.
func at(m *Model, ms int64) *Model {
	m.set.Now = func() time.Time { return time.UnixMilli(ms) }
	return m
}

func TestWorking(t *testing.T) {
	m := newModel(fixture(), nil)
	m.spinStyle = spinner("dots")
	if got := ansi.Strip(at(m, 0).working(m.st.muted, "thinking…")); got != "⠋ thinking…" {
		t.Errorf("working = %q", got)
	}
	m.spinStyle = spinner("off")
	if got := ansi.Strip(m.working(m.st.muted, "thinking…")); got != "thinking…" {
		t.Errorf("the spinner that does not move led the text: %q", got)
	}
	if got := ansi.Strip(m.spinner()); got != "⋯" {
		t.Errorf("off stands for a number by %q", got)
	}

	m.set.Shimmer = true
	a, b := at(m, 0).working(m.st.muted, "thinking…"), at(m, 6*shimmerStep.Milliseconds()).working(m.st.muted, "thinking…")
	if ansi.Strip(a) != "thinking…" || ansi.Strip(b) != "thinking…" {
		t.Errorf("the shimmer changed the text: %q, %q", ansi.Strip(a), ansi.Strip(b))
	}
	if a == b {
		t.Errorf("the shimmer did not move:\n%q", a)
	}
}

func TestWave(t *testing.T) {
	m := at(newModel(fixture(), nil), 0)
	m.spinStyle = spinner("dots")
	if m.spinnerAt(0) != m.spinnerAt(1) {
		t.Errorf("lines out of step without wave")
	}
	m.set.Wave = true
	if m.spinnerAt(0) == m.spinnerAt(1) || m.spinnerAt(1) != "⠙" || m.spinner() != "⠋" {
		t.Errorf("wave: %q %q, count %q", m.spinnerAt(0), m.spinnerAt(1), m.spinner())
	}
}
