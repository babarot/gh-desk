package app

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/babarot/gh-desk/internal/config"
	"github.com/babarot/gh-desk/internal/logging"
	"github.com/babarot/gh-desk/internal/model"
	"github.com/babarot/gh-desk/internal/release"
	"github.com/babarot/gh-desk/internal/tui"
	"github.com/babarot/gh-desk/internal/version"
)

// releaseOf is what the TUI tells of gh-desk's own releases, or nil when
// it tells of none: a build that is not a release, release.check off, or
// GH_NO_EXTENSION_UPDATE_NOTIFIER set. A release build keeps the version
// that ran whether or not it tells, so turning the check on later does not
// tell of an update long past.
func releaseOf(cfg config.File, e env, log *slog.Logger) *tui.Release {
	if !e.release {
		return nil
	}
	store := release.Open(e.stateDir)
	rec, err := store.Load()
	if err != nil {
		log.Warn("could not read what was found of the releases", logging.KeyOp, "release", "err", err.Error())
		rec = release.Record{}
	}
	running := version.Version
	from := ""
	if rec.LastRun != "" && release.Newer(running, rec.LastRun) {
		from = rec.LastRun
	}
	if rec.LastRun != running {
		rec.LastRun = running
		if err := store.Save(rec); err != nil {
			log.Warn("could not keep the version that ran", logging.KeyOp, "release", "err", err.Error())
		}
	}
	if !cfg.Release.Check || !e.notify {
		log.Debug("releases are not read", logging.KeyOp, "release", "check", cfg.Release.Check, "notify", e.notify)
		return nil
	}
	now := time.Now()
	return &tui.Release{
		Running: running, How: release.MethodOf(realPath(e.exe), realPath(e.extensions)).How(),
		Latest: rec.Latest, Told: rec.Told, UpdatedFrom: from, Due: rec.Due(now),
		Source: &releases{gh: e.gh, store: store, rec: rec, now: time.Now},
	}
}

// realPath is p with its symbolic links followed, or p as it is when that
// fails, as for a directory that does not exist.
func realPath(p string) string {
	if p == "" {
		return p
	}
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real
	}
	return p
}

// releases reads gh-desk's releases from GitHub and keeps what was found.
type releases struct {
	gh interface {
		Releases(ctx context.Context) ([]model.Release, error)
	}
	store *release.Store
	rec   release.Record
	now   func() time.Time
}

func (r *releases) Releases(ctx context.Context) ([]model.Release, error) { return r.gh.Releases(ctx) }

func (r *releases) Checked(latest string) error {
	r.rec.Checked = r.now()
	if latest != "" {
		r.rec.Latest = latest
	}
	return r.store.Save(r.rec)
}

func (r *releases) Told(v string) error {
	r.rec.Told = v
	return r.store.Save(r.rec)
}
