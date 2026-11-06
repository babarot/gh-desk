package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/babarot/gh-desk/internal/config"
	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/release"
	"github.com/babarot/gh-desk/internal/version"
)

// releaseEnv is a release build installed as a gh extension, whose last
// run was lastRun.
func releaseEnv(t *testing.T, lastRun string) (env, *release.Store) {
	t.Helper()
	dir := t.TempDir()
	ext := filepath.Join(dir, "gh", "extensions")
	bin := filepath.Join(ext, "gh-desk", "gh-desk")
	if err := os.MkdirAll(filepath.Dir(bin), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, nil, 0o700); err != nil {
		t.Fatal(err)
	}
	// Run through a link, as gh-desk on PATH may be.
	link := filepath.Join(dir, "gh-desk")
	if err := os.Symlink(bin, link); err != nil {
		t.Fatal(err)
	}
	e := env{stateDir: filepath.Join(dir, "state"), exe: link, release: true, notify: true, extensions: ext}
	store := release.Open(e.stateDir)
	if lastRun != "" {
		if err := store.Save(release.Record{LastRun: lastRun}); err != nil {
			t.Fatal(err)
		}
	}
	return e, store
}

func TestReleaseOfABuildThatIsNotARelease(t *testing.T) {
	e, store := releaseEnv(t, "0.1.0")
	e.release = false
	if r := releaseOf(config.Default(), e, logging.Discard()); r == nil || r.Tell || !r.FromSource || r.How != release.Page || r.Running != version.Version {
		t.Errorf("release = %+v", r)
	}
	if rec, _ := store.Load(); rec.LastRun != "0.1.0" {
		t.Errorf("a build that is not a release kept %q", rec.LastRun)
	}
}

func TestReleaseOfAReleaseBuild(t *testing.T) {
	e, store := releaseEnv(t, "0.1.0")
	r := releaseOf(config.Default(), e, logging.Discard())
	if r == nil {
		t.Fatal("no release")
	}
	if !r.Tell || r.FromSource || r.Running != version.Version || r.UpdatedFrom != "0.1.0" || !r.Due || r.How != "gh extension upgrade desk" {
		t.Errorf("release = %+v", r)
	}
	if rec, _ := store.Load(); rec.LastRun != version.Version {
		t.Errorf("last run kept %q", rec.LastRun)
	}
	// The next launch was not updated.
	if r := releaseOf(config.Default(), e, logging.Discard()); r == nil || !r.Tell || r.UpdatedFrom != "" {
		t.Errorf("second launch = %+v", r)
	}

	src, ok := r.Source.(*releases)
	if !ok {
		t.Fatalf("source is %T", r.Source)
	}
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	src.now = func() time.Time { return at }
	if err := src.Checked("9.9.9"); err != nil {
		t.Fatal(err)
	}
	if err := src.Told("9.9.9"); err != nil {
		t.Fatal(err)
	}
	if err := src.Checked(""); err != nil {
		t.Fatal(err)
	}
	rec, _ := store.Load()
	if !rec.Checked.Equal(at) || rec.Latest != "9.9.9" || rec.Told != "9.9.9" || rec.LastRun != version.Version {
		t.Errorf("record = %+v", rec)
	}
}

// Turned off, nothing is told and nothing is read until the notes are
// opened, but the version that ran is kept, so turning it on later does
// not tell of an update long past.
func TestReleaseOfTurnedOff(t *testing.T) {
	for name, off := range map[string]func(*config.File, *env){
		"release.check": func(c *config.File, _ *env) { c.Release.Check = false },
		"gh's notifier": func(_ *config.File, e *env) { e.notify = false },
	} {
		t.Run(name, func(t *testing.T) {
			e, store := releaseEnv(t, "0.1.0")
			cfg := config.Default()
			off(&cfg, &e)
			r := releaseOf(cfg, e, logging.Discard())
			if r == nil || r.Tell || r.UpdatedFrom != "" || r.Due || r.How != "gh extension upgrade desk" {
				t.Errorf("release = %+v", r)
			}
			if rec, _ := store.Load(); rec.LastRun != version.Version {
				t.Errorf("last run kept %q", rec.LastRun)
			}
		})
	}
}
