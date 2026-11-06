package tui

import (
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// clawdScenes are every scene the themes can make: each theme with every
// choice of its parts.
func clawdScenes(t *testing.T) []*clawdScene {
	t.Helper()
	var out []*clawdScene
	for _, th := range clawdThemes() {
		parts := [][]func() []clawdFrame{th.open, th.making, th.ready, th.fail}
		n := 1
		for _, p := range parts {
			n *= max(len(p), 1)
		}
		for k := range n {
			// pick gives each part its choice, in the order newScene asks.
			rest := k
			pick := func(n int) int { i := rest % n; rest /= n; return i }
			out = append(out, newScene(th, pick))
		}
	}
	return out
}

// Every scene draws Clawd in lines as wide as the scene's width all
// through, keeps each frame at least as long as the screen is drawn
// while Clawd moves, and ends holding what its rest holds: nothing held
// comes or goes as the scene comes to rest (ADR-090).
func TestClawdScenesHoldTogether(t *testing.T) {
	st := newStyles(theme("default"))
	all := clawdScenes(t)
	if len(all) < len(clawdThemes()) {
		t.Fatalf("%d scenes", len(all))
	}
	for _, s := range all {
		w := s.width()
		for _, part := range [][]clawdFrame{s.open, s.making, s.ready, s.fail} {
			for i, f := range part {
				if f.d < clawdStep {
					t.Errorf("%s: frame %d kept %v, under %v", s.theme, i, f.d, clawdStep)
				}
			}
		}
		for _, part := range [][]clawdFrame{s.open, s.making, s.ready, s.fail, {s.readyRest, s.failRest}} {
			for i, f := range part {
				for j, L := range st.clawdLines(f, w) {
					if got := ansi.StringWidth(L); got != w {
						t.Errorf("%s: frame %d line %d is %d wide, want %d", s.theme, i, j, got, w)
					}
				}
			}
		}
		for _, end := range []struct {
			m    []clawdFrame
			rest clawdFrame
		}{{s.ready, s.readyRest}, {s.fail, s.failRest}} {
			if len(end.m) > 0 && end.m[len(end.m)-1].held.thing() != end.rest.held.thing() {
				t.Errorf("%s: ends holding %q, rests holding %q", s.theme, end.m[len(end.m)-1].held.thing(), end.rest.held.thing())
			}
		}
	}
}

// A scene plays its opening, then its motion while the worktree is made,
// round after round; the end shows once the motion going on comes to an
// end, and Clawd then stands still in the end's rest.
func TestClawdSceneTimeline(t *testing.T) {
	d := 100 * time.Millisecond
	f := func(p pose) []clawdFrame { return stand(p).lasting(d) }
	s := &clawdScene{
		open:   motion(f(pArmsUp), f(pArmsUp)),
		making: motion(f(pLeft), f(pLeft), f(pLeft)),
		ready:  f(pose{eyes: eyesWink}), fail: f(pClosed),
		readyRest: stand(pRight), failRest: stand(pOneUp),
	}
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	for _, c := range []struct {
		name       string
		el, ended  time.Duration
		ok         bool
		want       pose
		wantMoving bool
	}{
		{"opening", ms(50), -1, true, pArmsUp, true},
		{"made", ms(250), -1, true, pLeft, true},
		{"made, rounds on", ms(1250), -1, true, pLeft, true},
		{"ended in the opening, which goes on", ms(150), ms(50), true, pArmsUp, true},
		{"then the end", ms(210), ms(50), true, pose{eyes: eyesWink}, true},
		{"then its rest", ms(350), ms(50), true, pRight, false},
		{"ended in a round, which goes on", ms(450), ms(350), false, pLeft, true},
		{"then the failure", ms(510), ms(350), false, pClosed, true},
		{"then its rest", ms(700), ms(350), false, pOneUp, false},
		{"ended at a round's end", ms(510), ms(500), true, pose{eyes: eyesWink}, true},
	} {
		got, moving := s.at(c.el, c.ended, c.ok)
		if got.pose != c.want || moving != c.wantMoving {
			t.Errorf("%s: %+v moving %v, want %+v moving %v", c.name, got.pose, moving, c.want, c.wantMoving)
		}
	}
	// With nothing to do while made, the end shows when it comes.
	still := &clawdScene{ready: f(pose{eyes: eyesWink}), readyRest: stand(pRight)}
	if got, moving := still.at(ms(10), -1, true); got.pose != pDefault || moving {
		t.Errorf("a scene with nothing to do moves: %+v", got.pose)
	}
	if got, _ := still.at(ms(30), ms(20), true); got.pose != (pose{eyes: eyesWink}) {
		t.Errorf("the end does not show when it comes: %+v", got.pose)
	}
}

// A scene that holds something takes the cells for it from the start; one
// that holds nothing does not.
func TestClawdSceneWidth(t *testing.T) {
	for _, s := range clawdScenes(t) {
		holds := false
		for _, part := range [][]clawdFrame{s.open, s.making, s.ready, s.fail, {s.readyRest, s.failRest}} {
			for _, f := range part {
				holds = holds || f.held != heldNone
			}
		}
		if want := map[bool]int{false: clawdW, true: clawdW + clawdHeldW}[holds]; s.width() != want {
			t.Errorf("%s: %d wide, want %d", s.theme, s.width(), want)
		}
	}
}

// The theme picked is not the one picked last.
func TestClawdThemeNotTwiceInARow(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	for _, pick := range []func(int) int{func(int) int { return 0 }, func(n int) int { return n - 1 }} {
		m.set.Pick, m.lastTheme = pick, -1
		last := m.pickScene().theme
		for range 5 {
			s := m.pickScene().theme
			if s == last {
				t.Fatalf("%s picked twice in a row", s)
			}
			last = s
		}
	}
}

// Clawd stands still when the spinner chosen does not move: in its
// standing pose while the worktree is made, in the rest of the end once
// it came.
func TestClawdStillWithTheSpinnerOff(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	m.set.Pick = func(n int) int { return min(1, n-1) } // the worker
	m.lastTheme = -1
	c := &chatScreen{scene: m.pickScene(), sceneAt: now}
	m.set.Now = func() time.Time { return now.Add(time.Second) }
	m.spinStyle = spinner("dots")
	if _, moving := m.clawdNow(c); !moving {
		t.Errorf("Clawd stands still while the worktree is made")
	}
	m.spinStyle = spinner("off")
	if f, moving := m.clawdNow(c); moving || f.pose != pDefault || f.held != heldNone {
		t.Errorf("Clawd moves with the spinner off: %+v", f)
	}
	c.endedAt, c.prepareErr = now.Add(500*time.Millisecond), "git fetch failed"
	if f, moving := m.clawdNow(c); moving || f != c.scene.failRest {
		t.Errorf("Clawd is not in the failure's rest: %+v", f)
	}
}
