package tui

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/babarot/gh-desk/internal/tui/keymap"
)

// The keys page of the documentation is written from the keymap and the
// sections of the ? popup, so it tells the same keys under the same
// titles as gh-desk does.

const keysReferencePath = "../../docs/reference/keys.md"

const keysReferenceHead = `# Keys

<!-- Generated from internal/tui/keymap by TestKeysReferenceIsUpToDate. Do not edit; run go test ./internal/tui -run TestKeysReferenceIsUpToDate -update. -->

` + "`?`" + ` lists every key, for the screen it is opened from: that screen's section comes first and unfolded. The line at the bottom of the screen shows the keys that matter there. These are the default keys.

Keys are bound by the name of their action, and each action's keys can be changed under ` + "`[keys]`" + ` in the [config file](configuration.md): a key or a list of keys, replacing the action's own, or ` + "`[]`" + ` to turn it off. A key given to an action is taken from an action that has it by default where they meet. ` + "`ctrl+c`" + ` and ` + "`esc`" + ` are fixed.

` + "```toml" + `
[keys]
approve = "a"
merge = ["M", "ctrl+m"]
browse = []
` + "```" + `
`

func keysReference() string {
	km := keymap.Default()
	var b strings.Builder
	b.WriteString(keysReferenceHead)
	for _, sc := range helpScopes {
		fmt.Fprintf(&b, "\n## %s\n\n", capitalized(sc.title))
		b.WriteString("| Key | Action | Does |\n|---|---|---|\n")
		for _, a := range km.Actions(sc.s) {
			if !ready[a] {
				continue
			}
			fmt.Fprintf(&b, "| %s | `%s` | %s |\n", keyCells(km.Keys(a)), a, km.Help(a))
		}
		if sc.s == keymap.ScopeGlobal {
			fmt.Fprintf(&b, "| `%s` | | quit, from anywhere; cannot be changed |\n", keymap.Interrupt)
			fmt.Fprintf(&b, "| `%s` | | back one level, everywhere; cannot be changed |\n", keymap.Back)
		}
	}
	b.WriteString("\n## Mouse\n\n| | |\n|---|---|\n")
	for _, s := range (&Model{}).helpSections() {
		if s.title != "mouse" {
			continue
		}
		for _, r := range s.rows {
			fmt.Fprintf(&b, "| %s | %s |\n", r[0], r[1])
		}
	}
	return b.String()
}

func keyCells(ks []string) string {
	if len(ks) == 0 {
		return "(none)"
	}
	q := make([]string, len(ks))
	for i, k := range ks {
		q[i] = "`" + strings.ReplaceAll(k, "|", `\|`) + "`"
	}
	return strings.Join(q, " ")
}

func capitalized(s string) string { return strings.ToUpper(s[:1]) + s[1:] }

func TestKeysReferenceIsUpToDate(t *testing.T) {
	want := keysReference()
	got, err := os.ReadFile(keysReferencePath)
	if *update {
		if err := os.WriteFile(keysReferencePath, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err != nil || string(got) != want {
		t.Errorf("%s is not the keys of the keymap; run go test ./internal/tui -run TestKeysReferenceIsUpToDate -update", keysReferencePath)
	}
}
