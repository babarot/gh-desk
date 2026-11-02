package tui

import (
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/babarot/gh-triage/internal/tui/keymap"
)

// The terminal's cursor stands right after the text typed in each text
// box, where an input method shows the text being composed.
func TestCursorAfterTheTypedText(t *testing.T) {
	typed := []string{"a", "あ", "b"}
	for _, tc := range []struct {
		name string
		open []string
	}{
		{"comment", []string{"c"}},
		{"review", []string{"R"}},
		{"claude", []string{"C"}},
		{"chat", []string{"i", "i"}},
		{"filter", []string{"/"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := startWith(t, fixture(), keymap.Default())
			fakeClaudeOf(t, m)
			press(m, tc.open...)
			press(m, typed...)
			s, cur := m.render()
			if cur == nil {
				t.Fatal("no cursor")
			}
			lines := strings.Split(ansi.Strip(s), "\n")
			if cur.Y < 0 || cur.Y >= len(lines) {
				t.Fatalf("cursor on row %d of %d", cur.Y, len(lines))
			}
			if left := ansi.Truncate(lines[cur.Y], cur.X, ""); !strings.HasSuffix(left, strings.Join(typed, "")) {
				t.Errorf("left of the cursor: %q", left)
			}
		})
	}
}

// No cursor is shown while nothing is typed in.
func TestNoCursorOnTheList(t *testing.T) {
	m, _ := startWith(t, fixture(), keymap.Default())
	if _, cur := m.render(); cur != nil {
		t.Errorf("cursor at %+v", cur)
	}
}

// inputPackages make text boxes that draw a cursor of their own unless
// told not to.
var inputPackages = map[string]bool{
	"charm.land/bubbles/v2/textarea":  true,
	"charm.land/bubbles/v2/textinput": true,
}

// TestTextBoxesFromNewInput fails on a text box made, or its cursor
// changed, outside newInput, which leaves the cursor to the terminal: a
// box that draws its own makes an input method show the text being
// composed away from it. A box made there still has to give its cursor to
// the view; TestCursorAfterTheTypedText checks that for each.
func TestTextBoxesFromNewInput(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	fset := gotoken.NewFileSet()
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		names := map[string]bool{}
		for _, im := range f.Imports {
			p, _ := strconv.Unquote(im.Path.Value)
			if !inputPackages[p] {
				continue
			}
			name := p[strings.LastIndex(p, "/")+1:]
			if im.Name != nil {
				name = im.Name.Name
			}
			names[name] = true
		}
		for _, decl := range f.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok && fd.Recv == nil && fd.Name.Name == "newInput" {
				continue
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if x, ok := sel.X.(*ast.Ident); ok && names[x.Name] && sel.Sel.Name == "New" {
					t.Errorf("%s: a text box is made with newInput", fset.Position(n.Pos()))
				}
				if sel.Sel.Name == "SetVirtualCursor" {
					t.Errorf("%s: only newInput sets where a text box's cursor is drawn", fset.Position(n.Pos()))
				}
				return true
			})
		}
	}
}
