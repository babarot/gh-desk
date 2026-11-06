package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/config"

	"github.com/babarot/gh-desk/internal/model"
)

// releasesRepo is the repository whose releases gh-desk reads: its own.
const releasesRepo = "babarot/gh-desk"

// releasesPage is how many releases one reading takes, newest first.
const releasesPage = 30

// ExtensionsDir is the directory gh installs its extensions in.
func ExtensionsDir() string { return filepath.Join(config.DataDir(), "extensions") }

type releaseResponse struct {
	TagName     string    `json:"tag_name"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
}

// Releases reads gh-desk's releases on github.com, newest first, leaving
// out drafts and pre-releases. Logged in to github.com, it reads with that
// login; logged in to another host only, it reads without one, since the
// releases are public.
func (c *Client) Releases(ctx context.Context) ([]model.Release, error) {
	path := fmt.Sprintf("repos/%s/releases?per_page=%d", releasesRepo, releasesPage)
	var resp []releaseResponse
	if c.host == "github.com" {
		if err := c.rest.DoWithContext(ctx, http.MethodGet, path, nil, &resp); err != nil {
			return nil, err
		}
	} else if err := c.readPublic(ctx, "https://api.github.com/"+path, &resp); err != nil {
		return nil, err
	}
	var out []model.Release
	for _, r := range resp {
		if r.Draft || r.Prerelease {
			continue
		}
		out = append(out, model.Release{Version: strings.TrimPrefix(r.TagName, "v"), Published: r.PublishedAt, Body: r.Body, URL: r.HTMLURL})
	}
	return out, nil
}

// readPublic reads url from api.github.com without a login.
func (c *Client) readPublic(ctx context.Context, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	hc := &http.Client{Transport: transport{next: c.next, log: c.log}, Timeout: 30 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}
