package claude

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
)

// A question is logged when it is asked and when it is answered, with the
// pull request and how many tools Claude used; what was asked, what
// Claude said and the tools' arguments are not.
func TestAskIsLoggedWithoutWhatIsSaid(t *testing.T) {
	dir := t.TempDir()
	streamFile := filepath.Join(dir, "stream")
	if err := os.WriteFile(streamFile, []byte(stream), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "claude")
	script := "#!/bin/sh\ncat > /dev/null\ncat \"" + streamFile + "\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	c := New(Options{Bin: bin, Model: "opus", Tools: []string{"Read", "Grep"}, Language: "English", Log: logging.New(&buf, slog.LevelInfo)})
	k := model.PRKey{Owner: "acme", Repo: "tool", Number: 7}
	if _, err := c.Ask(context.Background(), Request{PR: k, Dir: t.TempDir(), Session: "s-1", First: true, Prompt: "Is the secret plan safe?"}, func(Step) {}); err != nil {
		t.Fatal(err)
	}
	log := buf.String()
	for _, want := range []string{`"msg":"claude asked"`, `"msg":"claude answered"`, `"pr":"acme/tool#7"`, `"op":"ask"`, `"tools":2`} {
		if !strings.Contains(log, want) {
			t.Errorf("%s not in the log:\n%s", want, log)
		}
	}
	for _, never := range []string{"secret plan", "Looks fine", "/w/main.go", "TODO"} {
		if strings.Contains(log, never) {
			t.Errorf("%q is in the log:\n%s", never, log)
		}
	}
}
