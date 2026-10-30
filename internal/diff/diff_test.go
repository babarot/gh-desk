package diff

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return out
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Patches as git gives them, with names quoted as it quotes them by
// default and not.
func TestParseGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	dir := t.TempDir()
	write(t, dir, "a.txt", "1\n2\n3\n4\n5\n")
	write(t, dir, "日本語.md", "あ\n")
	write(t, dir, "sp ace.txt", "x\n")
	write(t, dir, "gone.txt", "g\n")
	write(t, dir, "old name.txt", "1\n2\n3\n4\n5\n6\n7\n8\n")
	write(t, dir, "bin.dat", "a\x00b")
	write(t, dir, "run.sh", "echo\n")
	git(t, dir, "init", "-q", "-b", "main")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "init")

	write(t, dir, "a.txt", "1\ntwo\n3\n4\n5")
	write(t, dir, "日本語.md", "い\n")
	write(t, dir, "sp ace.txt", "y\n")
	os.Remove(filepath.Join(dir, "gone.txt"))
	git(t, dir, "mv", "old name.txt", "new name.txt")
	write(t, dir, "new name.txt", "1\n2\n3\n4\n5\n6\n7\neight\n")
	write(t, dir, "bin.dat", "a\x00c")
	if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, dir, "added.txt", "n\n")
	git(t, dir, "add", "-N", "added.txt")

	for _, quote := range []string{"true", "false"} {
		out := git(t, dir, "-c", "core.quotePath="+quote, "diff-index", "-p", "-M", "--no-color", "--no-ext-diff", "HEAD")
		files := map[string]File{}
		for _, f := range Parse(out) {
			files[f.Path()] = f
		}
		check := func(path string, ok func(File) bool) {
			t.Helper()
			f, found := files[path]
			if !found {
				var names []string
				for p := range files {
					names = append(names, p)
				}
				t.Errorf("quotePath=%s: no %q in %q", quote, path, names)
				return
			}
			if !ok(f) {
				t.Errorf("quotePath=%s: %s: %+v", quote, path, f)
			}
		}
		check("a.txt", func(f File) bool {
			a, d := f.Stat()
			last := f.Hunks[len(f.Hunks)-1].Lines
			return a == 2 && d == 2 && last[len(last)-1].NoNewline && last[len(last)-1].Text == "5" &&
				f.Hunks[0].Lines[1].Text == "2" && f.Hunks[0].Lines[1].Old == 2 && f.Hunks[0].Lines[2].New == 2
		})
		check("日本語.md", func(f File) bool { a, d := f.Stat(); return a == 1 && d == 1 })
		check("sp ace.txt", func(f File) bool { return f.OldPath == "sp ace.txt" })
		check("gone.txt", func(f File) bool { a, d := f.Stat(); return f.NewPath == "" && a == 0 && d == 1 })
		check("new name.txt", func(f File) bool { a, d := f.Stat(); return f.OldPath == "old name.txt" && a == 1 && d == 1 })
		check("bin.dat", func(f File) bool { return f.Binary && len(f.Hunks) == 0 })
		check("run.sh", func(f File) bool { return f.OldMode == "100644" && f.NewMode == "100755" })
		check("added.txt", func(f File) bool { a, _ := f.Stat(); return f.OldPath == "" && a == 1 })
		if len(files) != 8 {
			t.Errorf("quotePath=%s: %d files", quote, len(files))
		}
	}
}

func TestGitPaths(t *testing.T) {
	for in, want := range map[string][2]string{
		"a/x.txt b/x.txt":                   {"x.txt", "x.txt"},
		"a/sp ace b/x b/sp ace b/x":         {"sp ace b/x", "sp ace b/x"},
		`"a/\346\227\245" "b/\346\227\245"`: {"日", "日"},
	} {
		a, b := gitPaths(in)
		if a != want[0] || b != want[1] {
			t.Errorf("%s: got %q %q", in, a, b)
		}
	}
}
