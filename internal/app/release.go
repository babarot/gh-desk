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

// releaseOf is what the TUI knows of gh-desk's own releases. Every build
// can open their notes from ?, which reads them then. A release build also
// tells of a newer release and of an update, unless release.check is off
// or GH_NO_EXTENSION_UPDATE_NOTIFIER is set; it keeps the version that ran
// whether or not it tells, so turning the check on later does not tell of
// an update long past. A build that is not a release is told the releases
// page as the way to update: whoever built it updates it.
func releaseOf(cfg config.File, e env, log *slog.Logger) *tui.Release {
	store := release.Open(e.stateDir)
	src := &releases{gh: e.gh, store: store, now: time.Now}
	r := &tui.Release{Running: version.Version, How: release.Page, FromSource: !e.release, Source: src}
	if !e.release {
		return r
	}
	rec, err := store.Load()
	if err != nil {
		log.Warn("could not read what was found of the releases", logging.KeyOp, "release", "err", err.Error())
		rec = release.Record{}
	}
	from := ""
	if rec.LastRun != "" && release.Newer(r.Running, rec.LastRun) {
		from = rec.LastRun
	}
	if rec.LastRun != r.Running {
		rec.LastRun = r.Running
		if err := store.Save(rec); err != nil {
			log.Warn("could not keep the version that ran", logging.KeyOp, "release", "err", err.Error())
		}
	}
	src.rec = rec
	r.How = release.MethodOf(realPath(e.exe), realPath(e.extensions)).How()
	if !cfg.Release.Check || !e.notify {
		log.Debug("releases are not told of", logging.KeyOp, "release", "check", cfg.Release.Check, "notify", e.notify)
		return r
	}
	r.Tell, r.Latest, r.Told, r.UpdatedFrom, r.Due = true, rec.Latest, rec.Told, from, rec.Due(time.Now())
	return r
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
