package listcache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/model"
)

func TestPutAndOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gh-desk", "lists.json")
	c, err := Open(path, "github.com", "me")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.List("q"); ok {
		t.Fatal("an empty cache has a list")
	}
	read := time.Date(2026, 10, 30, 19, 42, 0, 0, time.UTC)
	pr := model.PR{Key: model.PRKey{Owner: "acme", Repo: "api", Number: 1}, Title: "Add retries", HeadOID: "h1", HeavyHead: "h1"}
	if err := c.PutList(List{Query: "q", Total: 1, Read: read, PRs: []model.PR{pr}}); err != nil {
		t.Fatal(err)
	}
	if err := c.PutInbox(Inbox{Read: read, Notifs: []model.Notification{{ThreadID: "1", Key: pr.Key}}}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, %v", fi.Mode(), err)
	}

	again, err := Open(path, "github.com", "me")
	if err != nil {
		t.Fatal(err)
	}
	l, ok := again.List("q")
	if !ok || len(l.PRs) != 1 || l.PRs[0].Title != "Add retries" || !l.PRs[0].HeavyRead() || !l.Read.Equal(read) {
		t.Errorf("list %+v, %v", l, ok)
	}
	if in, ok := again.Inbox(); !ok || len(in.Notifs) != 1 {
		t.Errorf("inbox %+v, %v", in, ok)
	}
}

func TestAnotherAccountIsNotUsed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lists.json")
	a, _ := Open(path, "github.com", "alice")
	if err := a.PutList(List{Query: "q", PRs: []model.PR{{Key: model.PRKey{Owner: "acme", Repo: "api", Number: 1}, Title: "alice's"}}}); err != nil {
		t.Fatal(err)
	}
	b, err := Open(path, "github.com", "bob")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := b.List("q"); ok {
		t.Error("bob sees alice's list")
	}
	if ghe, _ := Open(path, "ghe.example.com", "alice"); func() bool { _, ok := ghe.List("q"); return ok }() {
		t.Error("another host sees the list")
	}
	// bob's write replaces the file whole: nothing of alice's stays.
	if err := b.PutList(List{Query: "other"}); err != nil {
		t.Fatal(err)
	}
	a2, _ := Open(path, "github.com", "alice")
	if _, ok := a2.List("q"); ok {
		t.Error("alice's list was kept in bob's file")
	}
}

// A write keeps the lists of other searches, written by a gh-desk with
// another config.
func TestWriteKeepsOtherLists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lists.json")
	one, _ := Open(path, "github.com", "me")
	two, _ := Open(path, "github.com", "me")
	if err := one.PutList(List{Query: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := two.PutList(List{Query: "b"}); err != nil {
		t.Fatal(err)
	}
	c, _ := Open(path, "github.com", "me")
	_, okA := c.List("a")
	_, okB := c.List("b")
	if !okA || !okB {
		t.Errorf("a %v, b %v", okA, okB)
	}
}

func TestBrokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lists.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Open(path, "github.com", "me")
	if err == nil {
		t.Error("a broken file gave no error")
	}
	if _, ok := c.List("q"); ok {
		t.Error("a broken file gave a list")
	}
	if err := c.PutList(List{Query: "q"}); err != nil {
		t.Errorf("a broken file is not written over: %v", err)
	}
}
