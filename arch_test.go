package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// The packages and what each may import, from the bottom layer up. A
// package imports only from lower layers and only the outside modules
// listed for it; the parts of the standard library that reach outside the
// process belong to one package each. docs/architecture.md shows this
// table, and TestArchitectureDoc keeps the two the same. Test files are
// not checked: a test may import what helps it.
//
// A package not in the table fails the test, so a new one is placed here
// on purpose.
var layers = []layer{
	{"internal/model", 0, nil, nil, "Shared types: pull requests and their names"},
	{"internal/logging", 0, nil, nil, "The slog logger and the log file"},
	{"internal/config", 1, []string{"model"}, []string{"github.com/pelletier/go-toml/v2"}, "Reading the config file and reporting its mistakes"},
	{"internal/keymap", 1, nil, nil, "Keys bound to actions by name"},
	{"internal/plan", 1, []string{"model"}, nil, "Deciding how to merge each pull request; selection rules"},
	{"internal/diff", 1, nil, nil, "Reading patches and the words that changed"},
	{"internal/github", 2, []string{"model", "logging"}, []string{"github.com/cli/go-gh/v2"}, "GitHub's GraphQL and REST APIs"},
	{"internal/state", 2, []string{"model", "logging"}, nil, "The pending-merge record and its lock"},
	{"internal/claude", 2, []string{"model", "logging"}, nil, "Running claude -p and the pull request worktrees"},
	{"internal/runner", 3, []string{"model", "logging", "plan"}, nil, "Bulk approve and merge, auto-merge watching, resuming"},
	{"internal/tui", 4, []string{"model", "logging", "plan", "diff", "keymap", "runner"},
		[]string{"charm.land/bubbletea/v2", "charm.land/bubbles/v2", "charm.land/lipgloss/v2", "github.com/charmbracelet/x/ansi", "github.com/alecthomas/chroma/v2"}, "The terminal UI"},
	{"internal/app", 5, []string{"model", "logging", "config", "keymap", "plan", "diff", "github", "state", "claude", "runner", "tui"}, nil, "Putting it together: flags, config, log, and starting the TUI"},
	{"", 6, []string{"app"}, nil, "Calls app.Run"},
}

// layer is a package's place: its layer, the internal packages (named
// without internal/) and outside modules it may import, and what it does.
type layer struct {
	pkg      string
	layer    int
	internal []string
	external []string
	does     string
}

// reach are the parts of the standard library that reach outside the
// process, and the packages allowed to use each.
var reach = map[string][]string{
	"net":      {"internal/github"},
	"net/http": {"internal/github"},
	"os/exec":  {"internal/claude", "internal/github", "internal/tui"},
	"syscall":  {"internal/state"},
}

const module = "github.com/babarot/gh-triage"

type goPackage struct {
	ImportPath string
	Imports    []string
}

func listPackages(t *testing.T) []goPackage {
	t.Helper()
	out, err := exec.Command("go", "list", "-json", "./...").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	var pkgs []goPackage
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var p goPackage
		if err := dec.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}

func TestArchitecture(t *testing.T) {
	for _, p := range listPackages(t) {
		rel := strings.TrimPrefix(strings.TrimPrefix(p.ImportPath, module), "/")
		i := slices.IndexFunc(layers, func(l layer) bool { return l.pkg == rel })
		if i < 0 {
			t.Errorf("%s is not in the layer table in arch_test.go and docs/architecture.md", p.ImportPath)
			continue
		}
		l := layers[i]
		for _, imp := range p.Imports {
			switch {
			case strings.HasPrefix(imp, module+"/"):
				dep := strings.TrimPrefix(imp, module+"/internal/")
				if !slices.Contains(l.internal, dep) {
					t.Errorf("%s imports %s, which is not in its allowed imports", p.ImportPath, imp)
				}
			case strings.Contains(strings.SplitN(imp, "/", 2)[0], "."):
				if !slices.ContainsFunc(l.external, func(m string) bool { return imp == m || strings.HasPrefix(imp, m+"/") }) {
					t.Errorf("%s imports %s, an outside module not allowed for it", p.ImportPath, imp)
				}
			default:
				if who, ok := reach[imp]; ok && !slices.Contains(who, rel) {
					t.Errorf("%s imports %s, which only %s may use", p.ImportPath, imp, strings.Join(who, ", "))
				}
			}
		}
	}
}

// TestLayersGoDownward checks the table itself: every allowed import is a
// package of a lower layer.
func TestLayersGoDownward(t *testing.T) {
	layerOf := map[string]int{}
	for _, l := range layers {
		layerOf[strings.TrimPrefix(l.pkg, "internal/")] = l.layer
	}
	for _, l := range layers {
		for _, dep := range l.internal {
			dl, ok := layerOf[dep]
			if !ok || dl >= l.layer {
				t.Errorf("%s (layer %d) may import %s (layer %d, known %v)", l.pkg, l.layer, dep, dl, ok)
			}
		}
	}
}

var update = flag.Bool("update", false, "rewrite the layer table in docs/architecture.md")

const (
	docPath  = "docs/architecture.md"
	docStart = "<!-- layers:start -->"
	docEnd   = "<!-- layers:end -->"
)

// docTable is the layer table as docs/architecture.md shows it.
func docTable() string {
	var b strings.Builder
	b.WriteString("| Layer | Package | Does | May import | Outside modules |\n|---|---|---|---|---|\n")
	for _, l := range layers {
		name := "`" + l.pkg + "`"
		if l.pkg == "" {
			name = "`main`"
		}
		join := func(ss []string) string {
			if len(ss) == 0 {
				return "-"
			}
			q := make([]string, len(ss))
			for i, s := range ss {
				q[i] = "`" + s + "`"
			}
			return strings.Join(q, ", ")
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s |\n", l.layer, name, l.does, join(l.internal), join(l.external))
	}
	b.WriteString("\nStandard library packages that reach outside the process:\n\n| Package | Only in |\n|---|---|\n")
	names := make([]string, 0, len(reach))
	for n := range reach {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		who := make([]string, len(reach[n]))
		for i, w := range reach[n] {
			who[i] = "`" + w + "`"
		}
		fmt.Fprintf(&b, "| `%s` | %s |\n", n, strings.Join(who, ", "))
	}
	return b.String()
}

func TestArchitectureDoc(t *testing.T) {
	src, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(src)
	start, end := strings.Index(doc, docStart), strings.Index(doc, docEnd)
	if start < 0 || end < start {
		t.Fatalf("%s has no %s ... %s block", docPath, docStart, docEnd)
	}
	want := docStart + "\n" + docTable() + docEnd
	got := doc[start : end+len(docEnd)]
	if got == want {
		return
	}
	if *update {
		if err := os.WriteFile(docPath, []byte(doc[:start]+want+doc[end+len(docEnd):]), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Errorf("the layer table in %s differs from arch_test.go; run go test -run TestArchitectureDoc -update", docPath)
}
