// Package ui makes the page of gh desk config ui, one HTML file that
// holds the settings, the config file as it was read, and the code, and
// serves it on this machine until it gives a file back. Nothing here writes
// the config file: gh desk config apply does (ADR-099).
package ui

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"html/template"
	"strings"

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

//go:embed web/prs.json
var prsJSON string

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
		"CSS":    template.CSS(appCSS),
		"Data":   template.JS(data),
		"Vendor": template.JS(htmJS),
		"App":    template.JS(modelJS + "\n" + tuiJS + "\n" + appJS),
	})
	return b.Bytes(), err
}

func dataJS(in Input, file map[string]any) (string, error) {
	var b strings.Builder
	for _, v := range []struct {
		name string
		v    any
	}{{"DATA", config.UI()}, {"KEYS", in.Keys}, {"THEMES", in.Themes}, {"SPIN", in.Spinners}, {"BOOT", boot{in.File.Path, file}}} {
		j, err := json.Marshal(v.v)
		if err != nil {
			return "", err
		}
		b.WriteString("const " + v.name + "=" + string(j) + ";\n")
	}
	b.WriteString("const ICONS=" + strings.TrimSpace(iconsJSON) + ";\nconst PRS=" + strings.TrimSpace(prsJSON) + ";\n")
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
