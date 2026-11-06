package model

import "time"

// Release is a release of gh-desk on GitHub.
type Release struct {
	// Version is the release's tag without its leading v, such as "0.8.0".
	Version   string
	Published time.Time
	// Body is the release notes, Markdown as GitHub generated them.
	Body string
	URL  string
}
