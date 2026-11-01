package config

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite docs/config.schema.json")

const schemaPath = "../../docs/config.schema.json"

func TestSettingsAreTheFilesKeys(t *testing.T) {
	var names []string
	for _, tb := range tables {
		names = append(names, tb.Name)
		var keys []string
		for _, s := range settings {
			if table, key, _ := strings.Cut(s.Key, "."); table == tb.Name {
				keys = append(keys, key)
			}
		}
		if got := settingsIn([]string{tb.Name}); !sameSet(keys, got) {
			t.Errorf("[%s]: the table has %v, doc has %v", tb.Name, keys, got)
		}
	}
	for name, es := range elems {
		names = append(names, name)
		var keys []string
		for _, e := range es {
			keys = append(keys, e.Key)
		}
		if got := settingsIn([]string{name}); !sameSet(keys, got) {
			t.Errorf("[[%s]]: the table has %v, doc has %v", name, keys, got)
		}
	}
	names = append(names, "keys")
	if got := settingsIn(nil); !sameSet(names, got) {
		t.Errorf("tables: %v, doc has %v", names, got)
	}
	for _, s := range settings {
		if _, _, ok := strings.Cut(s.Key, "."); !ok || s.Doc == "" {
			t.Errorf("%q has no table or no words", s.Key)
		}
	}
}

func sameSet(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}

// TestTemplateValuesAreTheDefaults uncomments every setting the template
// writes at its default and reads them as the defaults.
func TestTemplateValuesAreTheDefaults(t *testing.T) {
	examples := map[string]bool{}
	for _, s := range settings {
		if s.Example != "" {
			_, key, _ := strings.Cut(s.Key, ".")
			examples[key] = true
		}
	}
	var b strings.Builder
	n := 0
	inTables, inExample := false, false
	for line := range strings.SplitSeq(Template(""), "\n") {
		switch {
		case line == "[keys]":
			inTables = false
		case strings.HasPrefix(line, "["):
			inTables = true
			b.WriteString(line + "\n")
		case strings.HasPrefix(line, "# [["):
			// The examples of [[tabs]] and [[select]] run to a blank line.
			inExample = true
		case line == "":
			inExample = false
		case inTables && !inExample && strings.HasPrefix(line, "# ") && strings.Contains(line, " = "):
			key, _, _ := strings.Cut(strings.TrimPrefix(line, "# "), " = ")
			if strings.Contains(key, " ") || examples[key] {
				continue
			}
			b.WriteString(strings.TrimPrefix(line, "# ") + "\n")
			n++
		}
	}
	src := b.String()
	cfg, _, err := Load(write(t, src))
	if err != nil {
		t.Fatalf("%v\n%s", err, src)
	}
	if want := len(settings) - len(examples); n < want {
		t.Errorf("uncommented %d settings, want %d:\n%s", n, want, src)
	}
	// Compared as the template writes them, where an empty list and none
	// are the same.
	def := Default()
	for _, s := range settings {
		if got, want := s.Kind.toml(s.Value(cfg)), s.Kind.toml(s.Value(def)); got != want {
			t.Errorf("%s = %s, want %s", s.Key, got, want)
		}
	}
}

func TestSchemaIsUpToDate(t *testing.T) {
	want := Schema()
	got, err := os.ReadFile(schemaPath)
	if *update {
		if err := os.WriteFile(schemaPath, want, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err != nil || string(got) != string(want) {
		t.Errorf("%s is not the schema of the settings; run go test ./internal/config -run TestSchemaIsUpToDate -update", schemaPath)
	}
}

func TestEveryKindRejectsWhatItDoesNotTake(t *testing.T) {
	for _, s := range settings {
		table, key, _ := strings.Cut(s.Key, ".")
		var bad string
		switch k := s.Kind.(type) {
		case enumKind:
			bad = `"zzz"`
		case intKind:
			bad = fmt.Sprint(k.Min - 1)
		case stringKind:
			if !k.NonEmpty {
				continue
			}
			bad = `" "`
		case listKind:
			if len(k.Values) == 0 {
				continue
			}
			bad = `["zzz"]`
		case durationKind, daysKind:
			bad = `"soon"`
		case globsKind:
			bad = `["["]`
		default:
			continue
		}
		t.Run(s.Key, func(t *testing.T) {
			wantErrs(t, fmt.Sprintf("[%s]\n%s = %s\n", table, key, bad), s.Key)
		})
	}
}
