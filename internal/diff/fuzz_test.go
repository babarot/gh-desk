package diff

import "testing"

// A patch comes from GitHub, written by whoever opened the pull request.
// Reading one never panics, whatever it holds.
func FuzzParse(f *testing.F) {
	f.Add([]byte("diff --git a/x.go b/x.go\nindex 1..2 100644\n--- a/x.go\n+++ b/x.go\n@@ -1,2 +1,2 @@ func main() {\n-old\n+new\n same\n"))
	f.Add([]byte("diff --git a/a b/b\nrename from a\nrename to b\n"))
	f.Add([]byte("diff --git a/bin b/bin\nBinary files a/bin and b/bin differ\n"))
	f.Add([]byte("@@ -x,y +1 @@\n+\n"))
	f.Add([]byte(""))
	f.Fuzz(func(t *testing.T, b []byte) {
		Parse(b)
	})
}

// The words marked in two lines are spans within each line, in order and
// apart.
func FuzzInline(f *testing.F) {
	f.Add("return a + b", "return a - b")
	f.Add("日本語の行", "日本語の列")
	f.Add("", "added")
	f.Add("same", "same")
	f.Fuzz(func(t *testing.T, old, new string) {
		o, n := Inline(old, new)
		checkSpans(t, "old", old, o)
		checkSpans(t, "new", new, n)
	})
}

func checkSpans(t *testing.T, which, line string, spans []Span) {
	t.Helper()
	end := 0
	for _, s := range spans {
		if s.Start < end || s.Start >= s.End || s.End > len(line) {
			t.Fatalf("%s spans %v of %q are not in order within the line", which, spans, line)
		}
		end = s.End
	}
}
