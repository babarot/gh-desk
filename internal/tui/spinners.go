package tui

import (
	"maps"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// spinnerStyle is a spinner: its frames, each one cell wide, and how long
// each is shown.
type spinnerStyle struct {
	frames []string
	every  time.Duration
	// still is set on the spinner that does not move. It leads no text:
	// what is going on is written with a … of its own.
	still bool
}

// pingpong is frames followed by them back down, without repeating the
// ends.
func pingpong(frames string) []string {
	f := strings.Split(frames, "")
	for i := len(f) - 2; i > 0; i-- {
		f = append(f, f[i])
	}
	return f
}

// spinners are the spinners by name. A frame takes one cell on any
// terminal, also where East Asian Ambiguous characters are drawn wide, so
// that a spinner in a column keeps the columns in line; ◐, the block
// elements of a bar and • are left out for that.
var spinners = map[string]spinnerStyle{
	"dots":    {frames: strings.Split("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏", ""), every: 100 * time.Millisecond},
	"dense":   {frames: strings.Split("⣾⣽⣻⢿⡿⣟⣯⣷", ""), every: 100 * time.Millisecond},
	"bounce":  {frames: strings.Split("⠁⠂⠄⡀⢀⠠⠐⠈", ""), every: 100 * time.Millisecond},
	"snake":   {frames: strings.Split("⠁⠉⠙⠚⠒⠂⠂⠒⠲⠴⠤⠄⠄⠤⠴⠲⠒⠂⠂⠒⠚⠙⠉⠁", ""), every: 80 * time.Millisecond},
	"jump":    {frames: strings.Split("⢄⢂⢁⡁⡈⡐⡠", ""), every: 100 * time.Millisecond},
	"fill":    {frames: pingpong("⣀⣄⣤⣦⣶⣷⣿"), every: 90 * time.Millisecond},
	"sand":    {frames: strings.Split("⠁⠂⠄⡀⡈⡐⡠⣀⣁⣂⣄⣌⣔⣤⣥⣦⣮⣶⣷⣿⡿⠿⢟⠟⡛⠛⠫⢋⠋⠍⡉⠉⠑⠡⢁", ""), every: 80 * time.Millisecond},
	"line":    {frames: []string{"|", "/", "-", `\`}, every: 130 * time.Millisecond},
	"arc":     {frames: strings.Split("◜◠◝◞◡◟", ""), every: 100 * time.Millisecond},
	"quarter": {frames: strings.Split("◴◷◶◵", ""), every: 150 * time.Millisecond},
	"block":   {frames: strings.Split("▖▘▝▗", ""), every: 150 * time.Millisecond},
	"star":    {frames: pingpong("✶✸✹✺"), every: 120 * time.Millisecond},
	// off does not move. Its frame is ⋯, not …, which is East Asian
	// Ambiguous. The screen is still drawn every second while something
	// is read, for the seconds counted beside Claude's answers.
	"off": {frames: []string{"⋯"}, every: time.Second, still: true},
}

// claudeSpinner is Claude's own spinner, turned where Claude is answering
// about a pull request, which ends in ✦ when the answer comes. Not one of
// spinners: it is not chosen in the config file (ADR-041).
var claudeSpinner = spinnerStyle{frames: strings.Split("⋅✢✳✶✻✶✳✢", ""), every: 120 * time.Millisecond}

// claudeStill is claudeSpinner's frame when the spinner chosen does not
// move.
const claudeStill = "✻"

// claudeFrame is claudeSpinner's frame now.
func (m *Model) claudeFrame() string {
	if m.spinStyle.still {
		return claudeStill
	}
	s := claudeSpinner
	return s.frames[int(m.set.Now().UnixMilli()/s.every.Milliseconds())%len(s.frames)]
}

// SpinnerNames are the names of the spinners, sorted.
func SpinnerNames() []string { return slices.Sorted(maps.Keys(spinners)) }

// spinner is the spinner named name, or dots if there is none by that
// name; the config file's check keeps that from happening.
func spinner(name string) spinnerStyle {
	if s, ok := spinners[name]; ok {
		return s
	}
	return spinners["dots"]
}

// spinner is the spinner's frame now.
func (m *Model) spinner() string {
	s := m.spinStyle
	return s.frames[int(m.set.Now().UnixMilli()/s.every.Milliseconds())%len(s.frames)]
}

// working is text, which says what is going on, drawn in st and led by the
// spinner; the spinner that does not move leads it by nothing. With
// Settings.Shimmer a light runs over the text.
func (m *Model) working(st lipgloss.Style, text string) string {
	out := st.Render(text)
	if m.set.Shimmer {
		out = m.shimmer(st, text)
	}
	if m.spinStyle.still {
		return out
	}
	return m.st.mauve.Render(m.spinner()) + " " + out
}

// steady is working without the shimmer, for a popup, where a light
// running over the text would be one more thing moving beside its rows.
func (m *Model) steady(st lipgloss.Style, text string) string {
	if m.spinStyle.still {
		return st.Render(text)
	}
	return m.st.mauve.Render(m.spinner()) + " " + st.Render(text)
}

// shimmerStep is how long the light of a shimmer stays on a character;
// the screen is drawn this often while something is read, when shimmer
// is on.
const shimmerStep = 45 * time.Millisecond

// shimmer is text drawn in st with a light on it: text's brightest color
// on one character, and mauve on those beside it. The light runs from
// before the text to past its end, then starts again.
func (m *Model) shimmer(st lipgloss.Style, text string) string {
	return m.shimmerParts(styled{text, st})
}

// styled is text and the style it is drawn in.
type styled struct {
	text string
	st   lipgloss.Style
}

// shimmerParts is parts drawn each in its style, with one light running
// over all of them as shimmer runs it over one text.
func (m *Model) shimmerParts(parts ...styled) string {
	const before, after = 4, 8
	var rs []rune
	var base []lipgloss.Style
	for _, p := range parts {
		for _, r := range p.text {
			rs = append(rs, r)
			base = append(base, p.st)
		}
	}
	at := int(m.set.Now().UnixMilli()/shimmerStep.Milliseconds())%(len(rs)+before+after) - before
	styleOf := func(i int) lipgloss.Style {
		switch d := max(i-at, at-i); {
		case d == 0:
			return m.st.text
		case d <= 2:
			return m.st.mauve
		}
		return base[i]
	}
	var b strings.Builder
	for i := 0; i < len(rs); {
		s, j := styleOf(i), i+1
		for j < len(rs) && styleOf(j).GetForeground() == s.GetForeground() {
			j++
		}
		b.WriteString(s.Render(string(rs[i:j])))
		i = j
	}
	return b.String()
}
