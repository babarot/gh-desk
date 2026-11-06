package tui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// Clawd, Claude's mascot, as the welcome of the claude pane draws it. Its
// body is drawn as Claude Code draws it, and the motions copied from
// Claude Code are its own; the things it holds and the motions with them
// are gh-desk's (ADR-090).

// clawdStep is how long Claude Code keeps a frame of its motions, the
// shortest frame a motion has.
const clawdStep = 60 * time.Millisecond

// clawdW is how wide Clawd's body is, and clawdHeldW how many cells
// beside it a scene that holds something takes as well.
const (
	clawdW     = 9
	clawdHeldW = 3
)

type clawdEyes uint8

const (
	eyesOpen clawdEyes = iota
	eyesLeft
	eyesRight
	eyesClosed
	eyesWink
)

type clawdArms uint8

const (
	armsDown clawdArms = iota
	armsUp
	armsOneUp
)

type clawdFeet uint8

const (
	feetBoth clawdFeet = iota
	feetLeft
	feetRight
)

// pose is how Clawd stands: where it looks, its arms and its feet.
type pose struct {
	eyes clawdEyes
	arms clawdArms
	feet clawdFeet
}

// ink is what a cell of Clawd is drawn as.
type ink uint8

const (
	inkNone  ink = iota
	inkBody      // Claude's orange
	inkEye       // the orange over Clawd's black, where its eyes are
	inkLid       // Clawd's black over the orange: a closed eye
	inkMetal     // a hammer's head, a flag's pole
	inkWood      // a hammer's handle
	inkSpark     // where a hammer hits
	inkCloth     // a flag's cloth, checked
	inkHem       // a flag's cloth, its loose end
	inkPoof      // dust as Clawd lands
)

type clawdCell struct {
	r   rune
	ink ink
}

// held is what Clawd holds in a frame, and where.
type held uint8

const (
	heldNone held = iota
	heldHammerUp
	heldHammerDown
	heldHammerHit
	heldHammerLying
	heldFlagA
	heldFlagB
	heldFlagC
)

// heldCell is a cell of what Clawd holds: row -1 is the line above
// Clawd, and columns from clawdW on are the cells beside it.
type heldCell struct {
	row, col int
	r        rune
	ink      ink
}

var (
	hammerDown = []heldCell{{1, 9, '▀', inkWood}, {1, 10, '█', inkMetal}, {2, 10, '▀', inkMetal}}
	flagPole   = []heldCell{{-1, 9, '▏', inkMetal}, {0, 9, '▏', inkMetal}}
)

// heldCells are the cells of h.
func heldCells(h held) []heldCell {
	switch h {
	case heldHammerUp:
		return []heldCell{{-1, 9, '█', inkMetal}, {-1, 10, '▌', inkMetal}, {0, 9, '▌', inkWood}}
	case heldHammerDown:
		return hammerDown
	case heldHammerHit:
		return append(append([]heldCell{}, hammerDown...), heldCell{2, 11, '*', inkSpark}, heldCell{1, 11, '⋅', inkSpark})
	case heldHammerLying:
		return []heldCell{{2, 9, '▄', inkWood}, {2, 10, '█', inkMetal}}
	case heldFlagA:
		return append(append([]heldCell{}, flagPole...), heldCell{-1, 10, '▚', inkCloth}, heldCell{-1, 11, '▞', inkCloth})
	case heldFlagB:
		return append(append([]heldCell{}, flagPole...), heldCell{-1, 10, '▞', inkCloth}, heldCell{-1, 11, '▚', inkCloth})
	case heldFlagC:
		return append(append([]heldCell{}, flagPole...), heldCell{-1, 10, '▚', inkCloth}, heldCell{-1, 11, '▘', inkHem})
	}
	return nil
}

// thing is what h is of: a hammer, a flag, or nothing.
func (h held) thing() string {
	switch h {
	case heldHammerUp, heldHammerDown, heldHammerHit, heldHammerLying:
		return "hammer"
	case heldFlagA, heldFlagB, heldFlagC:
		return "flag"
	}
	return ""
}

// clawdFrame is a frame of a motion: a pose, moved down by offset and
// right by x and cut at Clawd's box, with dust beside its feet while it
// is down, what it holds, and how long it is kept.
type clawdFrame struct {
	pose      pose
	offset, x int
	poof      rune
	held      held
	d         time.Duration
}

// cells are Clawd's body in p: three lines of clawdW cells.
func (p pose) cells() [3][clawdW]clawdCell {
	var out [3][clawdW]clawdCell
	for r := range out {
		for c := range out[r] {
			out[r][c] = clawdCell{' ', inkNone}
		}
	}
	put := func(row, col int, s string, k ink) int {
		for _, r := range s {
			if r != ' ' {
				out[row][col] = clawdCell{r, k}
			}
			col++
		}
		return col
	}
	type armsGlyphs struct{ r1L, r1R, r2L, r2R string }
	a := map[clawdArms]armsGlyphs{
		armsDown:  {" ▐", "", "▝▜", "█▀"},
		armsUp:    {"▗▟", "▄", " ▜", "█▘"},
		armsOneUp: {" ▐", "▄", "▝▜", "█▘"},
	}[p.arms]
	col := put(0, 0, a.r1L, inkBody)
	switch p.eyes {
	case eyesLeft:
		col = put(0, col, "▟███▟█", inkEye)
	case eyesRight:
		col = put(0, col, "█▟███▟", inkEye)
	case eyesClosed:
		col = put(0, col, "▂", inkLid)
		col = put(0, col, "███", inkBody)
		col = put(0, col, "▂", inkLid)
		col = put(0, col, "█", inkBody)
	case eyesWink:
		col = put(0, col, "▛███", inkEye)
		col = put(0, col, "▂", inkLid)
		col = put(0, col, "█", inkBody)
	default:
		col = put(0, col, "▛███▛█", inkEye)
	}
	put(0, col, a.r1R, inkBody)
	put(1, 0, a.r2L, inkBody)
	put(1, 2, "█████", inkBody)
	put(1, 7, a.r2R, inkBody)
	put(2, 0, map[clawdFeet]string{feetBoth: " ▝▝   ▝▝ ", feetLeft: " ▝▝      ", feetRight: "      ▝▝ "}[p.feet], inkBody)
	return out
}

// grid is f drawn in the four lines Clawd has, the line above it first,
// w cells wide: clawdW, or more for what it holds.
func (f clawdFrame) grid(w int) [4][]clawdCell {
	var g [4][]clawdCell
	for r := range g {
		g[r] = make([]clawdCell, w)
		for c := range g[r] {
			g[r][c] = clawdCell{' ', inkNone}
		}
	}
	body := f.pose.cells()
	for r := range 3 {
		for c := range clawdW {
			sr, sc := r-f.offset, c-f.x
			if sr >= 0 && sr < 3 && sc >= 0 && sc < clawdW {
				g[r+1][c] = body[sr][sc]
			}
		}
	}
	if f.poof != 0 && f.offset > 0 {
		g[3][0], g[3][clawdW-1] = clawdCell{f.poof, inkPoof}, clawdCell{f.poof, inkPoof}
	}
	for _, h := range heldCells(f.held) {
		if h.col < w {
			g[h.row+1][h.col] = clawdCell{h.r, h.ink}
		}
	}
	return g
}

// clawdLines are f drawn in its colors, w cells wide, the line above
// Clawd first.
func (st styles) clawdLines(f clawdFrame, w int) [4]string {
	style := map[ink]lipgloss.Style{
		inkBody: st.mascot, inkEye: st.mascotEye, inkLid: st.mascotLid,
		inkMetal: st.muted, inkWood: st.peach, inkSpark: st.yellow, inkCloth: st.cloth, inkHem: st.text, inkPoof: st.dim,
	}
	var out [4]string
	for i, row := range f.grid(w) {
		var b strings.Builder
		for j := 0; j < len(row); {
			k := j
			var run strings.Builder
			for k < len(row) && row[k].ink == row[j].ink {
				run.WriteRune(row[k].r)
				k++
			}
			if row[j].ink == inkNone {
				b.WriteString(run.String())
			} else {
				b.WriteString(style[row[j].ink].Render(run.String()))
			}
			j = k
		}
		out[i] = b.String()
	}
	return out
}

// Building motions.

func stand(p pose) clawdFrame { return clawdFrame{pose: p} }

func (f clawdFrame) down(n int) clawdFrame     { f.offset = n; return f }
func (f clawdFrame) right(n int) clawdFrame    { f.x = n; return f }
func (f clawdFrame) dust(r rune) clawdFrame    { f.poof = r; return f }
func (f clawdFrame) holding(h held) clawdFrame { f.held = h; return f }

// lasting is f kept for d.
func (f clawdFrame) lasting(d time.Duration) []clawdFrame { f.d = d; return []clawdFrame{f} }

// steps is f for n of Claude Code's frames.
func (f clawdFrame) steps(n int) []clawdFrame { return f.lasting(time.Duration(n) * clawdStep) }

func motion(parts ...[]clawdFrame) []clawdFrame {
	var m []clawdFrame
	for _, p := range parts {
		m = append(m, p...)
	}
	return m
}

var (
	pDefault = pose{}
	pArmsUp  = pose{arms: armsUp}
	pOneUp   = pose{arms: armsOneUp}
	pLeft    = pose{eyes: eyesLeft}
	pRight   = pose{eyes: eyesRight}
	pClosed  = pose{eyes: eyesClosed}
)

// The motions of Claude Code 2.1.319, frame by frame.

// crouch is Claude Code's landing: down a line, with dust.
func crouch() []clawdFrame {
	return motion(stand(pDefault).down(1).dust('⋅').steps(1), stand(pDefault).down(1).dust('~').steps(1))
}

// blink ends a motion of Claude Code's with the eyes shut a moment.
func blink() []clawdFrame { return motion(stand(pClosed).steps(1), stand(pDefault).steps(1)) }

func ccIdle() []clawdFrame {
	return motion(stand(pDefault).steps(12), stand(pRight).steps(5), stand(pLeft).steps(5))
}

func ccJump() []clawdFrame {
	return motion(crouch(), stand(pArmsUp).steps(3), stand(pDefault).steps(1), crouch(), stand(pArmsUp).steps(3), stand(pDefault).steps(1))
}

func ccCelebrate() []clawdFrame { return motion(ccJump(), stand(pDefault).down(1).steps(3)) }

func ccSkip() []clawdFrame {
	return motion(stand(pDefault).down(1).right(-clawdW).steps(1),
		stand(pArmsUp).right(-6).steps(2), stand(pDefault).right(-6).steps(1), stand(pDefault).down(1).right(-6).steps(1),
		stand(pArmsUp).right(-3).steps(2), stand(pDefault).right(-3).steps(1), stand(pDefault).down(1).right(-3).steps(1),
		stand(pArmsUp).steps(2), crouch(), stand(pDefault).steps(1))
}

func ccWaddle() []clawdFrame {
	m := stand(pRight).right(-clawdW).steps(1)
	for x := -6; x <= -1; x++ {
		feet := feetRight
		if x%2 == 0 {
			feet = feetLeft
		}
		m = append(m, stand(pose{eyes: eyesRight, feet: feet}).right(x).steps(2)...)
	}
	return motion(m, stand(pRight).steps(2), stand(pDefault).steps(2), blink())
}

func ccDrop() []clawdFrame {
	return motion(stand(pArmsUp).down(-3).steps(1), stand(pArmsUp).down(-2).steps(2), stand(pArmsUp).down(-1).steps(2),
		stand(pArmsUp).steps(1), crouch(), stand(pDefault).steps(3), blink())
}

func ccPeekaboo() []clawdFrame {
	return motion(stand(pDefault).down(3).steps(1), stand(pDefault).down(2).steps(3), stand(pRight).down(2).steps(3),
		stand(pLeft).down(2).steps(3), stand(pDefault).down(2).steps(2), stand(pDefault).down(1).steps(1),
		stand(pArmsUp).steps(4), stand(pDefault).steps(2), blink())
}

func ccWink() []clawdFrame {
	return motion(stand(pose{eyes: eyesWink}).steps(5), stand(pDefault).steps(1))
}

func ccBoop() []clawdFrame {
	return motion(stand(pClosed).down(1).dust('⋅').steps(1), stand(pClosed).down(1).dust('~').steps(2),
		stand(pLeft).steps(4), stand(pDefault).steps(2), blink())
}

func ccSneeze() []clawdFrame {
	return motion(stand(pose{eyes: eyesClosed, arms: armsUp}).steps(4), stand(pClosed).down(1).dust('~').steps(2),
		stand(pClosed).steps(2), stand(pDefault).steps(2), blink())
}

// gh-desk's own motions.

// clawdTick is the unit the lengths of gh-desk's own frames are written in.
const clawdTick = time.Millisecond

// sunk is Clawd with its eyes shut, down a line.
func sunk() clawdFrame { return stand(pClosed).down(1) }

func hammering() []clawdFrame {
	return motion(stand(pOneUp).holding(heldHammerUp).lasting(260*clawdTick), stand(pDefault).holding(heldHammerHit).lasting(140*clawdTick),
		stand(pDefault).holding(heldHammerDown).lasting(220*clawdTick), stand(pOneUp).holding(heldHammerUp).lasting(260*clawdTick),
		stand(pDefault).holding(heldHammerHit).lasting(140*clawdTick), stand(pClosed).holding(heldHammerDown).lasting(120*clawdTick),
		stand(pDefault).holding(heldHammerDown).lasting(200*clawdTick))
}

func flagWaving() []clawdFrame {
	var m []clawdFrame
	for range 2 {
		for _, h := range []held{heldFlagA, heldFlagB, heldFlagC} {
			m = append(m, stand(pOneUp).holding(h).lasting(200*clawdTick)...)
		}
	}
	return m
}

func hammerDropping() []clawdFrame {
	return motion(stand(pOneUp).holding(heldHammerUp).lasting(300*clawdTick), stand(pDefault).holding(heldHammerDown).lasting(160*clawdTick),
		stand(pClosed).holding(heldHammerLying).lasting(400*clawdTick), stand(pRight).holding(heldHammerLying).lasting(500*clawdTick),
		sunk().holding(heldHammerLying).lasting(400*clawdTick))
}

func watching() []clawdFrame {
	return motion(stand(pDefault).lasting(2200*clawdTick), stand(pRight).lasting(1100*clawdTick), stand(pDefault).lasting(900*clawdTick),
		stand(pClosed).lasting(110*clawdTick), stand(pDefault).lasting(1600*clawdTick), stand(pClosed).lasting(110*clawdTick), stand(pDefault).lasting(150*clawdTick), stand(pClosed).lasting(110*clawdTick))
}

func breathing() []clawdFrame {
	return motion(stand(pDefault).lasting(600*clawdTick), stand(pDefault).down(1).lasting(600*clawdTick))
}

func drooping() []clawdFrame {
	return motion(stand(pLeft).lasting(350*clawdTick), stand(pClosed).lasting(250*clawdTick), sunk().lasting(500*clawdTick))
}

// clawdTheme is a scene of Clawd's, which the welcome picks one of at random:
// what Clawd does as the pane opens, while the worktree is made, and when
// it is made or fails, then the frame it stays in. Each part of a scene
// is picked from its candidates; none is nothing done.
type clawdTheme struct {
	name                      string
	open, making, ready, fail []func() []clawdFrame
	readyRest, failRest       clawdFrame
}

// clawdThemes are Clawd's scenes. A thing held in a scene stays: a motion that
// ends holding something ends in the rest that holds it.
func clawdThemes() []clawdTheme {
	return []clawdTheme{
		{
			name: "calm", making: []func() []clawdFrame{watching, breathing},
			ready: []func() []clawdFrame{ccWink}, readyRest: stand(pRight),
			fail: []func() []clawdFrame{drooping}, failRest: stand(pClosed),
		},
		{
			name: "worker", open: []func() []clawdFrame{ccWaddle, ccSkip}, making: []func() []clawdFrame{hammering},
			ready: []func() []clawdFrame{flagWaving}, readyRest: stand(pOneUp).holding(heldFlagA),
			fail: []func() []clawdFrame{hammerDropping}, failRest: sunk().holding(heldHammerLying),
		},
		{
			name: "claude code", open: []func() []clawdFrame{ccDrop, ccPeekaboo, ccSkip}, making: []func() []clawdFrame{ccIdle},
			ready: []func() []clawdFrame{ccCelebrate, ccJump}, readyRest: stand(pDefault),
			fail: []func() []clawdFrame{ccBoop, ccSneeze}, failRest: stand(pDefault),
		},
	}
}

// clawdScene is a theme with its parts picked.
type clawdScene struct {
	theme                     string
	open, making, ready, fail []clawdFrame
	readyRest, failRest       clawdFrame
}

// newScene picks t's parts with pick, which gives a number below n.
func newScene(t clawdTheme, pick func(n int) int) *clawdScene {
	one := func(c []func() []clawdFrame) []clawdFrame {
		if len(c) == 0 {
			return nil
		}
		return c[pick(len(c))]()
	}
	return &clawdScene{theme: t.name, open: one(t.open), making: one(t.making), ready: one(t.ready), fail: one(t.fail),
		readyRest: t.readyRest, failRest: t.failRest}
}

// width is how wide s draws Clawd: wider, from the start, when it holds
// anything, so that the text beside Clawd stays put.
func (s *clawdScene) width() int {
	for _, m := range [][]clawdFrame{s.open, s.making, s.ready, s.fail, {s.readyRest, s.failRest}} {
		for _, f := range m {
			if f.held != heldNone {
				return clawdW + clawdHeldW
			}
		}
	}
	return clawdW
}

func length(m []clawdFrame) time.Duration {
	var d time.Duration
	for _, f := range m {
		d += f.d
	}
	return d
}

// frameIn is the frame of m at t from its start.
func frameIn(m []clawdFrame, t time.Duration) clawdFrame {
	for _, f := range m {
		if t < f.d {
			return f
		}
		t -= f.d
	}
	return m[len(m)-1]
}

// at is the frame of s el after it began, and whether Clawd is moving
// then. ended is when the worktree was made or failed, after the scene
// began, or negative while it is made; ok says it was made. The end
// shows once the motion going on comes to an end: the opening, or a round
// of the motion while it is made.
func (s *clawdScene) at(el, ended time.Duration, ok bool) (clawdFrame, bool) {
	openD, makeD := length(s.open), length(s.making)
	switchAt := time.Duration(-1)
	switch {
	case ended < 0:
	case ended <= openD:
		switchAt = openD
	case makeD == 0:
		switchAt = ended
	default:
		rounds := (ended - openD + makeD - 1) / makeD
		switchAt = openD + rounds*makeD
	}
	if switchAt < 0 || el < switchAt {
		switch {
		case el < openD:
			return frameIn(s.open, el), true
		case makeD == 0:
			return stand(pDefault), false
		}
		return frameIn(s.making, (el-openD)%makeD), true
	}
	end, rest := s.ready, s.readyRest
	if !ok {
		end, rest = s.fail, s.failRest
	}
	if t := el - switchAt; t < length(end) {
		return frameIn(end, t), true
	}
	return rest, false
}
