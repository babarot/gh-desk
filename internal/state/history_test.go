package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var historyStart = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

func addQuestion(t *testing.T, h *History, n int, q string) {
	t.Helper()
	e := HistoryEntry{ID: fmt.Sprintf("id-%d", n), At: historyStart.Add(time.Duration(n) * time.Minute), Tab: "review", Question: q}
	if err := h.Add(e); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryKeepsQuestionsAndAnswers(t *testing.T) {
	dir := t.TempDir()
	h := OpenHistory(dir, "github.com")
	addQuestion(t, h, 1, "bots with passing checks")
	addQuestion(t, h, 2, "先週から動いていないものは？")
	if err := h.Reply("id-1", "2 bots"); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		got, err := OpenHistory(dir, "github.com").Load()
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 || got[0].Question != "先週から動いていないものは？" || got[0].Reply != "" ||
			got[1].Question != "bots with passing checks" || got[1].Reply != "2 bots" || got[1].Tab != "review" {
			t.Fatalf("got %+v", got)
		}
	}
	p := filepath.Join(dir, "triage", "github.com", "history.jsonl")
	if fi, err := os.Stat(p); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("file %v, %v", fi, err)
	}
}

func TestHistoryOfNothing(t *testing.T) {
	got, err := OpenHistory(t.TempDir(), "").Load()
	if err != nil || len(got) != 0 {
		t.Errorf("got %+v, %v", got, err)
	}
}

// The newest of a question stays, with its own answer, and the file is
// written back as that.
func TestHistoryKeepsTheNewestOfAQuestion(t *testing.T) {
	dir := t.TempDir()
	h := OpenHistory(dir, "")
	addQuestion(t, h, 1, "same")
	if err := h.Reply("id-1", "old answer"); err != nil {
		t.Fatal(err)
	}
	addQuestion(t, h, 2, "same")
	got, err := h.Load()
	if err != nil || len(got) != 1 || got[0].ID != "id-2" || got[0].Reply != "" {
		t.Fatalf("got %+v, %v", got, err)
	}
	b, _ := os.ReadFile(h.path())
	if strings.Count(string(b), "\n") != 1 {
		t.Errorf("file:\n%s", b)
	}
}

func TestHistoryLimit(t *testing.T) {
	h := OpenHistory(t.TempDir(), "")
	for i := range historyLimit + 3 {
		addQuestion(t, h, i, fmt.Sprintf("q%d", i))
	}
	got, err := h.Load()
	if err != nil || len(got) != historyLimit {
		t.Fatalf("%d entries, %v", len(got), err)
	}
	if got[0].Question != fmt.Sprintf("q%d", historyLimit+2) || got[len(got)-1].Question != "q3" {
		t.Errorf("kept %q to %q", got[0].Question, got[len(got)-1].Question)
	}
}

// A line that cannot be read is left out, and one of another format keeps
// the file as it is.
func TestHistorySkipsWhatItCannotRead(t *testing.T) {
	h := OpenHistory(t.TempDir(), "")
	addQuestion(t, h, 1, "kept")
	f, err := os.OpenFile(h.path(), os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(f, "not json")
	fmt.Fprintln(f, `{"v":2,"id":"x","q":"from a newer gh-desk"}`)
	f.Close()
	got, err := h.Load()
	if err != nil || len(got) != 1 || got[0].Question != "kept" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if b, _ := os.ReadFile(h.path()); !strings.Contains(string(b), "from a newer gh-desk") {
		t.Errorf("a newer line was lost:\n%s", b)
	}
}

// Writes from several gh-desk are not lost to one that writes the file back.
func TestHistoryWritesTogether(t *testing.T) {
	dir := t.TempDir()
	var wg sync.WaitGroup
	const each = 40
	for w := range 4 {
		wg.Go(func() {
			h := OpenHistory(dir, "")
			for i := range each {
				n := w*each + i
				addQuestion(t, h, n, fmt.Sprintf("q%d", n))
				if _, err := h.Load(); err != nil {
					t.Error(err)
				}
			}
		})
	}
	wg.Wait()
	got, err := OpenHistory(dir, "").Load()
	if err != nil || len(got) != 4*each {
		t.Fatalf("%d entries, %v", len(got), err)
	}
}
