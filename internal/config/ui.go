package config

import (
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// UIKind is a kind of setting as the page of config ui takes it. Type is
// the kind's Go type, as it is the only name the kinds have.
type UIKind struct {
	Type     string   `json:"type"`
	Name     string   `json:"name"`
	Min      int      `json:"min,omitempty"`
	Max      int      `json:"max,omitempty"`
	Values   []string `json:"values,omitempty"`
	NonEmpty bool     `json:"nonEmpty,omitempty"`
	Off      bool     `json:"off,omitempty"`
}

// UISetting is a setting as the page takes it. Default is its value, left
// out when it has no value of its own.
type UISetting struct {
	Key     string `json:"key"`
	Table   string `json:"table"`
	Name    string `json:"name"`
	Doc     string `json:"doc"`
	Kind    UIKind `json:"kind"`
	Default any    `json:"defv,omitempty"`
	Example string `json:"example,omitempty"`
}

// UIElem is a key of an array of tables.
type UIElem struct {
	Key  string `json:"key"`
	Doc  string `json:"doc"`
	Kind UIKind `json:"kind"`
}

// UINamed is a table or an array of tables.
type UINamed struct {
	Name string `json:"name"`
	Doc  string `json:"doc"`
}

// UIData is the table of settings, for the page of config ui.
type UIData struct {
	Settings     []UISetting         `json:"settings"`
	Tables       []UINamed           `json:"tables"`
	Arrays       []UINamed           `json:"arrays"`
	Elems        map[string][]UIElem `json:"elems"`
	DefaultRules any                 `json:"defaultRules"`
	DefaultTabs  any                 `json:"defaultTabs"`
	Schema       string              `json:"schema"`
}

// UI is the table of settings as the page of config ui takes it.
func UI() UIData {
	def := Default()
	d := UIData{Elems: map[string][]UIElem{}, Schema: SchemaURL, DefaultRules: tomlTables(defaultSelect, "select"), DefaultTabs: tomlTables(uncommented(tabsWords), "tabs")}
	for _, s := range settings {
		table, name := splitKey(s.Key)
		u := UISetting{Key: s.Key, Table: table, Name: name, Doc: s.Doc, Kind: uiKind(s.Kind), Example: strings.Trim(s.Example, `"`)}
		if v := s.Kind.toml(s.Value(def)); s.Example == "" || v != `""` {
			u.Default = parsed(v)
		}
		d.Settings = append(d.Settings, u)
	}
	for _, t := range tables {
		d.Tables = append(d.Tables, UINamed{t.Name, t.Doc})
	}
	for _, a := range arrayTables {
		d.Arrays = append(d.Arrays, UINamed{a.Name, a.Doc})
		for _, e := range elems[a.Name] {
			d.Elems[a.Name] = append(d.Elems[a.Name], UIElem{e.Key, e.Doc, uiKind(e.Kind)})
		}
	}
	return d
}

func uiKind(k kind) UIKind {
	u := UIKind{Type: fmt.Sprintf("%T", k), Name: kindName(k)}
	switch v := k.(type) {
	case intKind:
		u.Min, u.Max = v.Min, v.Max
	case enumKind:
		u.Values = v.Values
	case listKind:
		u.Values, u.NonEmpty = v.Values, v.NonEmpty
	case durationKind:
		u.Off = v.Off
	case stringKind:
		u.NonEmpty = v.NonEmpty
	}
	return u
}

// parsed is v, written as the file takes it, read back; nil when it is
// none, as an optional bool that is not set writes.
func parsed(v string) any {
	var t struct {
		X any `toml:"x"`
	}
	if err := toml.Unmarshal([]byte("x = "+v), &t); err != nil {
		return nil
	}
	return t.X
}

func tomlTables(src, name string) any {
	var t map[string]any
	if err := toml.Unmarshal([]byte(src), &t); err != nil {
		panic(fmt.Sprintf("config: %s: %v", name, err))
	}
	return t[name]
}

// uncommented are the commented-out example lines of words.
func uncommented(words string) string {
	var b strings.Builder
	for line := range strings.Lines(words) {
		l, ok := strings.CutPrefix(line, "# ")
		if ok && (strings.HasPrefix(l, "[[") || isKeyLine(l)) {
			b.WriteString(l)
		}
	}
	return b.String()
}

func isKeyLine(l string) bool {
	k, _, ok := strings.Cut(l, " = ")
	return ok && k != "" && strings.Trim(k, "abcdefghijklmnopqrstuvwxyz_") == ""
}

// Raw is the file as written: the keys in it and nothing else, which a
// File cannot tell, since a key left out has its default there.
func Raw(src []byte) (map[string]any, error) {
	var t map[string]any
	if err := toml.Unmarshal(src, &t); err != nil {
		return nil, err
	}
	if t == nil {
		t = map[string]any{}
	}
	return t, nil
}
