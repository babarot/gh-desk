// Package ui makes the settings page, one HTML file that holds the
// settings, the config file as it was read, the lists last read and the
// code, and serves it on this machine until a file is saved. Replacing the
// file and judging a rule are given to it by the caller (ADR-101, ADR-102).
// The site shows the same page, as the settings playground, served by
// nothing (ADR-103).
package ui

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"html/template"
	"strings"
	"time"

	"github.com/babarot/gh-desk/internal/config"
)

//go:embed web/page.html
var pageHTML string

//go:embed web/app.css
var appCSS string

//go:embed web/htm.js
var htmJS string

//go:embed web/model.js
var modelJS string

//go:embed web/tui.js
var tuiJS string

//go:embed web/app.js
var appJS string

//go:embed web/icons.json
var iconsJSON string

//go:embed web/xterm.js
var xtermJS string

//go:embed web/xterm.css
var xtermCSS string

// SamplePRs are the made-up pull requests shown when no list was read yet,
// as model.PR in JSON, updated relative to SampleTime.
//
//go:embed web/prs.json
var SamplePRs []byte

// SampleTime is the time SamplePRs are as of.
var SampleTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// PR is a row as the page draws and counts it. Index tells it apart in what
// a rule is judged to.
type PR struct {
	Index    int    `json:"i"`
	Repo     string `json:"repo"`
	N        int    `json:"n"`
	Author   string `json:"author"`
	Bot      bool   `json:"bot,omitempty"`
	Title    string `json:"title"`
	Add      int    `json:"add"`
	Del      int    `json:"del"`
	Files    int    `json:"files"`
	Draft    bool   `json:"draft,omitempty"`
	State    string `json:"state"`
	Checks   string `json:"checks"`
	Decision string `json:"decision"`
	// Heavy says the size, checks and review were read for the head shown.
	Heavy   bool    `json:"heavy"`
	Group   string  `json:"group"`
	Why     string  `json:"why"`
	Review  string  `json:"review,omitempty"`
	Age     string  `json:"age"`
	Updated int     `json:"updated"`
	At      float64 `json:"at"`
}

// List is the rows of a tab, and when they were read.
type List struct {
	Read time.Time `json:"read"`
	PRs  []PR      `json:"prs"`
}

// Lists are what the preview draws: the lists last read by their search,
// or Sample, made up, with no list read.
type Lists struct {
	Viewer string          `json:"viewer"`
	Tabs   map[string]List `json:"tabs"`
	Inbox  *List           `json:"inbox"`
	Sample []PR            `json:"sample"`
}

// Key is a key action as the page lists it.
type Key struct {
	Action     string   `json:"action"`
	Keys       []string `json:"keys"`
	Scope      string   `json:"scope"`
	ScopeWords string   `json:"scopeWords"`
}

// File is the config file as it was read.
type File struct {
	// Path is where it is, as the page shows it.
	Path string
	// Src is what it holds; nil when there is none.
	Src []byte
}

// Input is what a page is made from.
type Input struct {
	File     File
	Keys     []Key
	Themes   any
	Spinners any
	Lists    Lists
	// Playground makes the settings playground: no gh-desk answers it, so it shows
	// only what needs none, and saves nothing.
	Playground bool
	// Live has the page show gh-desk itself, which the server's Terminal
	// runs, in place of the list drawn again (ADR-109).
	Live bool
}

type boot struct {
	Path string         `json:"path"`
	File map[string]any `json:"file"`
}

// Page is the page for in.
func Page(in Input) ([]byte, error) {
	file, err := config.Raw(in.File.Src)
	if err != nil {
		return nil, err
	}
	if in.File.Src != nil {
		file["__comments"] = commentLines(in.File.Src)
	}
	data, err := dataJS(in, file)
	if err != nil {
		return nil, err
	}
	t, err := template.New("page").Parse(pageHTML)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	err = t.Execute(&b, map[string]any{
		"CSS":      template.CSS(appCSS),
		"Data":     template.JS(data),
		"Vendor":   template.JS(htmJS),
		"App":      template.JS(modelJS + "\n" + tuiJS + "\n" + appJS),
		"Live":     in.Live && !in.Playground,
		"XtermCSS": template.CSS(xtermCSS),
		"Xterm":    template.JS(xtermJS),
	})
	return b.Bytes(), err
}

func dataJS(in Input, file map[string]any) (string, error) {
	var b strings.Builder
	for _, v := range []struct {
		name string
		v    any
	}{{"DATA", config.UI()}, {"KEYS", in.Keys}, {"THEMES", in.Themes}, {"SPIN", in.Spinners}, {"BOOT", boot{in.File.Path, file}}, {"LISTS", in.Lists}, {"PLAYGROUND", in.Playground}, {"LIVE", in.Live && !in.Playground}} {
		j, err := json.Marshal(v.v)
		if err != nil {
			return "", err
		}
		b.WriteString("const " + v.name + "=" + string(j) + ";\n")
	}
	b.WriteString("const ICONS=" + strings.TrimSpace(iconsJSON) + ";\n")
	return b.String(), nil
}

func commentLines(src []byte) int {
	n := 0
	for line := range strings.Lines(string(src)) {
		if l := strings.TrimSpace(line); l == "" || l[0] == '#' {
			n++
		}
	}
	return n
}
