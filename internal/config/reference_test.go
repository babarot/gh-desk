package config

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// The configuration page of the documentation is written from the same
// table as the template and the schema, so the three cannot disagree.
// Only the words around the table are written here.

const referencePath = "../../docs/reference/configuration.md"

const referenceHead = `# Configuration

<!-- Generated from internal/config by TestReferenceIsUpToDate. Do not edit; run go test ./internal/config -run TestReferenceIsUpToDate -update. -->

The first launch writes ` + "`~/.config/gh-desk/config.toml`" + ` (or under ` + "`$XDG_CONFIG_HOME`" + `) with no settings in it. A setting left out takes its default; write the ones you change. A mistake in the file is reported with where it is, and ` + "`gh desk config check`" + ` reports them without starting.

` + "```sh" + `
gh desk config reference    # every setting, as this page lists them
gh desk config check        # read the file and print its mistakes
gh desk config show --diff  # the settings that differ from the defaults
gh desk config edit         # open it in $EDITOR, then check it
gh desk config schema       # the JSON Schema of the file
` + "```" + `

The keys are under ` + "`[keys]`" + `; see [Keys](keys.md).
`

// Reference is the configuration page: the arrays of tables first, as the
// file starts with them, then every table and its settings.
func Reference() string {
	def := Default()
	var b strings.Builder
	b.WriteString(referenceHead)

	b.WriteString("\n## [[tabs]]\n\n")
	b.WriteString(mdText("The tabs, in the order shown; the keys 1 to 9 go to them. Tabs given replace the default four, and one of them must be the inbox. At most "+fmt.Sprint(MaxTabs)+" tabs.") + "\n\n")
	b.WriteString("```toml\n" + defaultTabs + "```\n")
	writeElems(&b, "tabs")

	b.WriteString("\n## [[select]]\n\n")
	b.WriteString(mdText(`The rules S offers to select pull requests, besides the built-in "approved by me". A pull request matches a rule when it meets every key the rule gives. These rules are the default; rules written in the file replace all of them, and `+"`select = []`"+` at the top of the file leaves none:`) + "\n\n")
	b.WriteString("```toml\n" + defaultSelect + "```\n")
	writeElems(&b, "select")

	for _, t := range tables {
		fmt.Fprintf(&b, "\n## [%s]\n", t.Name)
		if t.Doc != "" {
			b.WriteString("\n" + mdText(t.Doc) + "\n")
		}
		for _, s := range settings {
			table, _ := splitKey(s.Key)
			if table != t.Name {
				continue
			}
			v := s.Example
			label := "Default"
			if v == "" {
				v = s.Kind.toml(s.Value(def))
			} else {
				label = "Example"
			}
			fmt.Fprintf(&b, "\n### %s\n\n", s.Key)
			fmt.Fprintf(&b, "%s · %s: `%s`\n\n", kindName(s.Kind), label, v)
			b.WriteString(mdText(s.Doc) + "\n")
		}
	}
	return b.String()
}

func writeElems(b *strings.Builder, name string) {
	b.WriteString("\n| Key | Type | |\n|---|---|---|\n")
	for _, e := range elems[name] {
		fmt.Fprintf(b, "| `%s` | %s | %s |\n", e.Key, kindName(e.Kind), strings.ReplaceAll(mdText(e.Doc), "|", `\|`))
	}
}

// defaultTabs are the tabs of the template, uncommented.
var defaultTabs = func() string {
	_, tabs, _ := strings.Cut(tabsWords, "#\n# [[tabs]]")
	var b strings.Builder
	b.WriteString("[[tabs]]")
	for line := range strings.SplitSeq(tabs, "\n") {
		b.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "#"), " ") + "\n")
	}
	return strings.TrimLeft(strings.TrimRight(b.String(), "\n"), "\n") + "\n"
}()

// mdText is a setting's words as Markdown text: the words are written for
// a comment, so an angle bracket is a placeholder, not HTML.
func mdText(s string) string {
	return strings.NewReplacer("<", "&lt;", ">", "&gt;").Replace(s)
}

func TestReferenceIsUpToDate(t *testing.T) {
	want := Reference()
	got, err := os.ReadFile(referencePath)
	if *update {
		if err := os.WriteFile(referencePath, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err != nil || string(got) != want {
		t.Errorf("%s is not the reference of the settings; run go test ./internal/config -run TestReferenceIsUpToDate -update", referencePath)
	}
}
