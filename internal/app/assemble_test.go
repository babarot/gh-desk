package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/babarot/gh-triage/internal/config"
	"github.com/babarot/gh-triage/internal/github"
	"github.com/babarot/gh-triage/internal/logging"
	"github.com/babarot/gh-triage/internal/model"
	"github.com/babarot/gh-triage/internal/state"
)

// assembleIn runs assemble with the state and the cache in temporary
// directories, and closes what it opened when the test ends.
func assembleIn(t *testing.T, stateDir string, noCache bool) (parts, config.File) {
	t.Helper()
	gh, err := github.New(logging.Discard(), github.Options{Host: "github.com", AuthToken: "test-token"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Cache.Dir = t.TempDir()
	p := assemble(cfg, []string{"a warning"}, env{gh: gh, host: "github.com", viewer: "me", stateDir: stateDir,
		borrow: []string{"/src"}, noCache: noCache}, logging.Discard())
	t.Cleanup(func() {
		if p.store != nil {
			p.store.Close()
		}
		if p.lock != nil {
			p.lock.close()
		}
	})
	return p, cfg
}

// The first gh-triage keeps the record of pending merges and resumes what
// it holds.
func TestAssembleKeepsTheRecord(t *testing.T) {
	dir := t.TempDir()
	s, err := state.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	pending := model.Pending{Key: model.PRKey{Owner: "babarot", Repo: "gh-triage", Number: 1}, ID: "PR_1", Head: "abc123", Author: "me"}
	if err := s.Put(pending); err != nil {
		t.Fatal(err)
	}
	s.Close()

	p, cfg := assembleIn(t, dir, false)
	set := p.set
	if p.store == nil || p.lock != nil || set.Lock != nil || !set.CanWait || set.StoreProblem != "" {
		t.Errorf("store %v, lock %v, can wait %v, problem %q", p.store, p.lock, set.CanWait, set.StoreProblem)
	}
	if len(set.Resume) != 1 || set.Resume[0].Key != pending.Key {
		t.Errorf("resume = %+v", set.Resume)
	}
	if set.Viewer != "me" || set.Host != "github.com" || !slices.Equal(set.ConfigWarnings, []string{"a warning"}) {
		t.Errorf("viewer %q, host %q, warnings %v", set.Viewer, set.Host, set.ConfigWarnings)
	}
	if set.Chats == nil || set.Layout == nil || set.Lists == nil {
		t.Errorf("chats %v, layout %v, lists %v", set.Chats, set.Layout, set.Lists)
	}
	if p.cl.ws.Dir != cfg.CacheDir() || !slices.Equal(p.cl.ws.Borrow, []string{"/src"}) {
		t.Errorf("workspace in %q borrowing %v", p.cl.ws.Dir, p.cl.ws.Borrow)
	}
}

// Another gh-triage holding the record leaves this one without waiting
// merges, with a lock to take the record over once the other quits.
func TestAssembleWhileAnotherHoldsTheRecord(t *testing.T) {
	dir := t.TempDir()
	other, err := state.Open(dir)
	if err != nil || other == nil {
		t.Fatalf("store %v, err %v", other, err)
	}
	defer other.Close()

	p, _ := assembleIn(t, dir, false)
	if p.store != nil || p.lock == nil || p.set.Lock == nil || p.set.CanWait || p.set.StoreProblem != "" {
		t.Errorf("store %v, lock %v, can wait %v, problem %q", p.store, p.lock, p.set.CanWait, p.set.StoreProblem)
	}
}

// A record that cannot be read is shown as a problem, and nothing waits
// for it to be taken over.
func TestAssembleWithAnUnreadableRecord(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pending.json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, _ := assembleIn(t, dir, false)
	if p.store != nil || p.lock != nil || p.set.CanWait || !strings.Contains(p.set.StoreProblem, dir) {
		t.Errorf("store %v, lock %v, can wait %v, problem %q", p.store, p.lock, p.set.CanWait, p.set.StoreProblem)
	}
}

// --no-cache neither shows nor keeps the lists last read.
func TestAssembleWithoutTheCache(t *testing.T) {
	p, _ := assembleIn(t, t.TempDir(), true)
	if p.set.Lists != nil {
		t.Errorf("lists = %v", p.set.Lists)
	}
}
