package state

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/model"
)

func TestChats(t *testing.T) {
	dir := t.TempDir()
	c := OpenChats(dir, "github.com")
	k := model.PRKey{Owner: "babarot", Repo: "gh-desk", Number: 3}
	if _, ok, err := c.Load(k); ok || err != nil {
		t.Fatalf("Load before any = %v, %v", ok, err)
	}
	r := ChatRecord{Key: k, Session: "s1", Started: true, Dir: "/w", Head: "h1", Rev: 1,
		Turns: []ChatTurn{{You: true, Text: "Review this pull request.", Head: "h1", At: time.Unix(1, 0).UTC()},
			{Text: "Looks fine.", Steps: []ChatStep{{Tool: "Read", Arg: "main.go"}}, At: time.Unix(2, 0).UTC()}}}
	if err := c.Save(r); err != nil {
		t.Fatal(err)
	}
	got, ok, err := c.Load(k)
	if !ok || err != nil || got.Session != "s1" || got.Rev != 1 || len(got.Turns) != 2 || got.Turns[1].Steps[0].Arg != "main.go" {
		t.Fatalf("Load = %+v, %v, %v", got, ok, err)
	}
	if fi, err := os.Stat(filepath.Join(dir, "chats", "github.com", "babarot", "gh-desk", "3.json")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("file %v, %v; want mode 0600", fi, err)
	}
	keys, err := c.Keys()
	if err != nil || !slices.Equal(keys, []model.PRKey{k}) {
		t.Errorf("Keys = %v, %v", keys, err)
	}
	// Another host keeps its own.
	if keys, _ := OpenChats(dir, "ghe.example.com").Keys(); len(keys) != 0 {
		t.Errorf("another host has %v", keys)
	}
	if err := c.Remove(k); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := c.Load(k); ok {
		t.Error("removed, still there")
	}
}

func TestChatOfAnotherFormatIsNotRead(t *testing.T) {
	dir := t.TempDir()
	c := OpenChats(dir, "")
	k := model.PRKey{Owner: "babarot", Repo: "gh-desk", Number: 3}
	p := c.path(k)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(`{"version":99,"rev":3}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := c.Load(k); ok || err == nil {
		t.Errorf("Load = %v, %v; want an error", ok, err)
	}
}

func TestChatLock(t *testing.T) {
	c := OpenChats(t.TempDir(), "github.com")
	k := model.PRKey{Owner: "babarot", Repo: "gh-desk", Number: 3}
	unlock, held, _, err := c.Lock(k)
	if !held || err != nil {
		t.Fatalf("Lock = %v, %v", held, err)
	}
	// A lock belongs to the open file, so a second Lock in this process
	// contends as another process would.
	_, held2, who, err := c.Lock(k)
	if held2 || err != nil || who == "" {
		t.Fatalf("second Lock = %v, %q, %v; want refused, naming this process", held2, who, err)
	}
	// Removing the conversation leaves the lock.
	if err := c.Remove(k); err != nil {
		t.Fatal(err)
	}
	if _, held, _, _ := c.Lock(k); held {
		t.Fatal("the lock went with the conversation")
	}
	unlock()
	u, held, _, err := c.Lock(k)
	if !held || err != nil {
		t.Fatalf("Lock after unlock = %v, %v", held, err)
	}
	u()
}
