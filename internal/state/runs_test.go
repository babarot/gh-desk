package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunsSweepKeepsLiveRuns(t *testing.T) {
	r := Runs{Dir: filepath.Join(t.TempDir(), "runs")}
	live, release, err := r.New()
	if err != nil {
		t.Fatal(err)
	}
	// A run left by a gh-desk that did not exit cleanly: its directory
	// is there, its lock is free.
	dead := filepath.Join(r.Dir, runPrefix+"dead")
	young := filepath.Join(r.Dir, runPrefix+"young")
	for _, d := range []string{dead, young} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, runLock), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-2 * youngRun)
	if err := os.Chtimes(dead, old, old); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(r.Dir, "not-a-run")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	n, err := r.Sweep()
	if err != nil || n != 1 {
		t.Fatalf("Sweep = %d, %v; want 1", n, err)
	}
	for dir, want := range map[string]bool{live: true, dead: false, young: true, other: true} {
		if _, err := os.Stat(dir); (err == nil) != want {
			t.Errorf("%s there = %v, want %v", filepath.Base(dir), err == nil, want)
		}
	}
	release()
	if _, err := os.Stat(live); !os.IsNotExist(err) {
		t.Errorf("release left %s", live)
	}
}
