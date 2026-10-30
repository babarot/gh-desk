// Package tui is gh-triage's terminal UI. It reads keys through the keymap
// by action name, never by key, and learns about the world only through
// what the app package hands it.
package tui

import (
	"fmt"
	"log/slog"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/babarot/gh-triage/internal/keymap"
)

// Model is the TUI's state.
type Model struct {
	keys   keymap.Map
	log    *slog.Logger
	width  int
	height int
	help   bool
}

// New returns the TUI with keys bound by km, logging to log.
func New(km keymap.Map, log *slog.Logger) *Model {
	return &Model{keys: km, log: log}
}

// Run shows the TUI until it quits.
func Run(km keymap.Map, log *slog.Logger) error {
	_, err := tea.NewProgram(New(km, log)).Run()
	return err
}

// Init starts nothing yet.
func (m *Model) Init() tea.Cmd { return nil }

// Update handles a message.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		return m.key(msg.String())
	}
	return m, nil
}

// key handles a key press, as Bubble Tea writes it.
func (m *Model) key(k string) (tea.Model, tea.Cmd) {
	switch {
	case k == keymap.Interrupt:
		return m, tea.Quit
	case k == keymap.Back:
		if m.help {
			m.help = false
			return m, nil
		}
		return m, tea.Quit
	case m.keys.Matches("quit", k):
		m.log.Debug("key", "action", "quit")
		return m, tea.Quit
	case m.keys.Matches("help", k):
		m.log.Debug("key", "action", "help")
		m.help = !m.help
	}
	return m, nil
}

var (
	headerStyle = lipgloss.NewStyle().Background(lipgloss.Color("#3b3b52")).Foreground(lipgloss.Color("#b0b8d1"))
	appStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#96bfff")).Bold(true)
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#706f87"))
	keyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#96bfff")).Bold(true)
	mutedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#8888a4"))
)

// View draws the screen.
func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m *Model) render() string {
	var b strings.Builder
	header := appStyle.Render("triage") + dimStyle.Render(" // gh-triage")
	b.WriteString(headerStyle.Width(max(m.width, 1)).Render(header))
	b.WriteString("\n\n")
	if m.help {
		b.WriteString(m.renderHelp())
	} else {
		b.WriteString(mutedStyle.Render("  Nothing to show yet."))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(m.hints("quit", "help"))
	return b.String()
}

// hints is the key line: each action's first key and its words.
func (m *Model) hints(actions ...keymap.Action) string {
	var parts []string
	for _, a := range actions {
		ks := m.keys.Keys(a)
		if len(ks) == 0 {
			continue
		}
		parts = append(parts, keyStyle.Render(ks[0])+" "+mutedStyle.Render(m.keys.Help(a)))
	}
	return strings.Join(parts, dimStyle.Render(" · "))
}

// renderHelp lists every action with its keys, scope by scope.
func (m *Model) renderHelp() string {
	var b strings.Builder
	for _, s := range []keymap.Scope{keymap.ScopeGlobal, keymap.ScopeNav, keymap.ScopeList, keymap.ScopePR, keymap.ScopePopup, keymap.ScopeChat, keymap.ScopeCompose} {
		fmt.Fprintf(&b, "  %s\n", dimStyle.Render(string(s)))
		for _, a := range m.keys.Actions(s) {
			ks := m.keys.Keys(a)
			if len(ks) == 0 {
				continue
			}
			fmt.Fprintf(&b, "    %-14s %s\n", keyStyle.Render(strings.Join(ks, " ")), mutedStyle.Render(m.keys.Help(a)))
		}
	}
	return b.String()
}
