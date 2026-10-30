package state

import (
	"testing"
	"time"

	"github.com/babarot/gh-triage/internal/model"
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
