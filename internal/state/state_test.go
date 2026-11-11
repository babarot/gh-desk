package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/model"
)

func TestStore(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil || s == nil {
		t.Fatalf("Open = %v, %v", s, err)
	}
	// flock(2) locks belong to the open file, so a second Open in this
	// process contends as another process would.
	if other, err := Open(dir); err != nil || other != nil {
		t.Fatalf("second Open = %v, %v; want nil, nil while held", other, err)
	}
	if pid, wd, ok := Holder(dir); !ok || pid != os.Getpid() || wd == "" {
		t.Fatalf("Holder = %d, %q, %v; want this process", pid, wd, ok)
	}
	k := model.PRKey{Owner: "acme", Repo: "api", Number: 1}
	p := model.Pending{Key: k, ID: "PR_1", Head: "h1", Author: "renovate", AuthorIsBot: true, ChecksWaited: 3 * time.Minute}
	if err := s.Put(p); err != nil {
		t.Fatal(err)
	}
	p.Watching = true
	if err := s.Put(p); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s2, err := Open(dir)
	if err != nil || s2 == nil {
		t.Fatalf("reopen = %v, %v", s2, err)
	}
	got := s2.Pending()
	if len(got) != 1 || got[0].Key != k || !got[0].Watching || got[0].ChecksWaited != 3*time.Minute {
		t.Fatalf("pending = %+v", got)
	}
	if err := s2.Remove(k); err != nil || len(s2.Pending()) != 0 {
		t.Fatalf("remove: %v %+v", err, s2.Pending())
	}
	s2.Close()
}

func TestHolderUnknown(t *testing.T) {
	dir := t.TempDir()
	if _, _, ok := Holder(dir); ok {
		t.Fatal("Holder with no lock file reported one")
	}
	// An older gh-desk leaves the lock file empty.
	if err := os.WriteFile(filepath.Join(dir, "pending.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := Holder(dir); ok {
		t.Fatal("Holder of an empty lock file reported one")
	}
	if err := os.WriteFile(filepath.Join(dir, "pending.lock"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := Holder(dir); ok {
		t.Fatal("Holder of a broken lock file reported one")
	}
}

func TestLayout(t *testing.T) {
	dir := t.TempDir()
	l := OpenLayout(dir)
	if h, err := l.PreviewHeight(); err != nil || h != 0 {
		t.Fatalf("PreviewHeight before any = %d, %v; want 0, nil", h, err)
	}
	if err := l.SavePreviewHeight(20); err != nil {
		t.Fatal(err)
	}
	if h, err := OpenLayout(dir).PreviewHeight(); err != nil || h != 20 {
		t.Fatalf("PreviewHeight = %d, %v; want 20, nil", h, err)
	}
}

// The scope and the pane's height are kept apart: saving one keeps the
// other, and a value of another scope is not given back.
func TestLayoutScope(t *testing.T) {
	dir := t.TempDir()
	l := OpenLayout(dir)
	if v, err := l.Scope("owner"); err != nil || v != "" {
		t.Fatalf("Scope before any = %q, %v", v, err)
	}
	if err := l.SavePreviewHeight(12); err != nil {
		t.Fatal(err)
	}
	if err := l.SaveScope("owner", "acme"); err != nil {
		t.Fatal(err)
	}
	if err := l.SavePreviewHeight(14); err != nil {
		t.Fatal(err)
	}
	l = OpenLayout(dir)
	if v, err := l.Scope("owner"); err != nil || v != "acme" {
		t.Errorf("Scope(owner) = %q, %v; want acme", v, err)
	}
	if v, _ := l.Scope("repo"); v != "" {
		t.Errorf("Scope(repo) = %q; want none, as acme is an owner", v)
	}
	if h, _ := l.PreviewHeight(); h != 14 {
		t.Errorf("PreviewHeight = %d; want 14", h)
	}
}
