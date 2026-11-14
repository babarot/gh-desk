package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-desk/internal/tui/keymap"
)

func TestThumbAt(t *testing.T) {
	for _, tt := range []struct {
		first, room, total, from, to int
	}{
		{0, 10, 100, 0, 1},   // a small thumb at the top
		{90, 10, 100, 9, 10}, // at the very end
		{1, 10, 100, 1, 2},   // moved at all, it leaves the top
		{89, 10, 100, 8, 9},  // not at the end, it does not touch it
		{0, 10, 20, 0, 5},    // half shows, half the track
		{10, 10, 20, 5, 10},
		{5, 10, 20, 3, 8},
	} {
		from, to := thumbAt(tt.first, tt.room, tt.total)
		if from != tt.from || to != tt.to {
			t.Errorf("%+v: got %d, %d", tt, from, to)
		}
	}
}

// thumbRows are the rows of the screen with the thumb of a scrollbar in
// column x.
func thumbRows(m *Model, x int) []int {
	var rows []int
	for y, row := range strings.Split(screen(m), "\n") {
		if ansi.Cut(row, x, x+1) == "┃" {
			rows = append(rows, y)
		}
	}
	return rows
}

// The diff, longer than the pane, shows a scrollbar on the pane's right
// edge, which follows it down to the bottom; it takes no width from the
// lines.
func TestPaneScrollbar(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "v", "-", "-", "-", "-", "-")
	edge := m.width - 1
	top := thumbRows(m, edge)
	if len(top) == 0 {
		t.Fatalf("no scrollbar on the pane's edge:\n%s", screen(m))
	}
	for range 50 {
		press(m, "J")
	}
	end := thumbRows(m, edge)
	if len(end) == 0 || end[0] <= top[0] {
		t.Fatalf("the thumb did not move down: %v, then %v", top, end)
	}
	_, bottom := lineOf(t, m, "╰")
	if end[len(end)-1] != bottom-1 {
		t.Fatalf("scrolled to the end, the thumb ends on row %d, not above the bottom edge on %d", end[len(end)-1], bottom-1)
	}
}

// What fits shows no scrollbar.
func TestPaneNoScrollbarWhenAllShows(t *testing.T) {
	m := start(t, fixture(), keymap.Default())
	press(m, "v")
	if rows := thumbRows(m, m.width-1); len(rows) > 0 {
		t.Fatalf("a scrollbar on rows %v, though the diff fits:\n%s", rows, screen(m))
	}
}
