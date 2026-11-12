package tui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// On a screen too narrow for anything, the screen is still drawn: a value
// of the preview's facts, cut to what is left beside its key, left nothing
// to cut by and was cut forever, taking all the memory there was.
func TestDrawsOnANarrowScreen(t *testing.T) {
	for _, w := range []int{1, 4, 8, 12, 14, 16} {
		m, _ := startWith(t, fixture(), keymap.Default())
		m.Update(tea.WindowSizeMsg{Width: w, Height: 34})
		done := make(chan struct{})
		go func() {
			m.render()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			// A loop that does not end takes the memory with it: stop the
			// whole test binary, not only this test.
			panic(fmt.Sprintf("drawing a screen %d wide did not end", w))
		}
	}
}
