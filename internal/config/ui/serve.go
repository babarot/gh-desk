package ui

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// maxFile is the most a page may send: a config file is far smaller.
const maxFile = 1 << 20

// closeGrace is how long a page that went away has to come back, as one
// does when it is reloaded, before the command ends.
const closeGrace = 3 * time.Second

// Server serves one page, on this machine only, until it saves a file.
// A setting can name a command, so a web site open in the browser must not
// reach it: a site cannot read the token in the URL, and Host and Origin
// are checked, which keeps out one that renames itself to 127.0.0.1 in DNS.
// Another program of this user is not kept out: it can read the URL, and
// write the config file without any page. See ADR-101.
type Server struct {
	Page []byte
	// Save takes a file sent, and puts it in place. Its error is shown on
	// the page, which stays open to fix it. It may wait for an answer in
	// the terminal, until ctx is canceled; one runs at a time.
	Save func(ctx context.Context, src []byte) error

	token string
	host  string
	ctx   context.Context

	mu sync.Mutex
	// ended is true once done has its value.
	ended bool
	done  chan bool
	// saving is true during Save, and left that the page went meanwhile,
	// so that it ends once Save has not saved.
	saving bool
	left   bool
	// loads counts the times the page was read, and grace is how
	// long a page that went away has to be read again.
	loads int
	grace time.Duration
}

// Serve shows the page, calling open with its URL, and says whether a file
// was saved, or false when the page was closed without one. Canceling ctx
// stops it, and returns ctx's error.
func (s *Server) Serve(ctx context.Context, open func(url string)) (bool, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return false, err
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		l.Close()
		return false, err
	}
	s.token, s.host, s.ctx, s.done = hex.EncodeToString(b), l.Addr().String(), ctx, make(chan bool, 1)
	if s.grace == 0 {
		s.grace = closeGrace
	}
	srv := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(l) }()
	open("http://" + s.host + "/" + s.token + "/")

	var saved bool
	select {
	case saved = <-s.done:
	case <-ctx.Done():
		err = ctx.Err()
	}
	// Lets the answer to the page finish.
	stop, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(stop)
	// Canceled while a save was finishing: it is saved all the same.
	if err != nil {
		select {
		case saved = <-s.done:
			if saved {
				err = nil
			}
		default:
		}
	}
	return saved, err
}

// ServeHTTP answers only the page's own two requests.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Host != s.host {
		http.Error(w, "wrong host", http.StatusForbidden)
		return
	}
	path, ok := strings.CutPrefix(r.URL.Path, "/")
	token, rest, _ := strings.Cut(path, "/")
	if !ok || subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) != 1 {
		http.NotFound(w, r)
		return
	}
	switch {
	case r.Method == http.MethodGet && rest == "":
		h := w.Header()
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Cache-Control", "no-store")
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; img-src data:; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		h.Set("Referrer-Policy", "no-referrer")
		s.mu.Lock()
		s.loads++
		s.left = false
		s.mu.Unlock()
		_, _ = w.Write(s.Page)
	case r.Method == http.MethodPost && rest == "save":
		s.save(w, r)
	case r.Method == http.MethodPost && rest == "close":
		s.closePage(w, r)
	case r.Method == http.MethodPost && rest == "gone":
		s.pageGone(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) ownOrigin(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Origin") != "http://"+s.host {
		http.Error(w, "wrong origin", http.StatusForbidden)
		return false
	}
	return true
}

// finish ends Serve, once.
func (s *Server) finish(saved bool) {
	if !s.ended {
		s.ended = true
		s.done <- saved
	}
}

// leave ends Serve with nothing, or after Save if one is waiting: the
// terminal may still be asking, and an answer there is still an answer.
func (s *Server) leave() {
	if s.saving {
		s.left = true
		return
	}
	s.finish(false)
}

// closePage ends it with nothing, as the page's Close asks.
func (s *Server) closePage(w http.ResponseWriter, r *http.Request) {
	if !s.ownOrigin(w, r) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.leave()
}

// pageGone is told when the tab goes. A reload does that too, so it ends
// only if no page came back.
func (s *Server) pageGone(w http.ResponseWriter, r *http.Request) {
	if !s.ownOrigin(w, r) {
		return
	}
	s.mu.Lock()
	seen := s.loads
	s.mu.Unlock()
	time.AfterFunc(s.grace, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.loads == seen {
			s.leave()
		}
	})
}

// save holds no lock during Save, which can wait for the terminal for as
// long as the user takes: the page must still be able to go.
func (s *Server) save(w http.ResponseWriter, r *http.Request) {
	if !s.ownOrigin(w, r) {
		return
	}
	src, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxFile))
	if err != nil {
		http.Error(w, "too large", http.StatusRequestEntityTooLarge)
		return
	}
	s.mu.Lock()
	if s.ended || s.saving {
		s.mu.Unlock()
		refuse(w, http.StatusConflict, errors.New("a file is being saved already"))
		return
	}
	s.saving = true
	ctx := s.ctx
	s.mu.Unlock()

	err = s.Save(ctx, src)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.saving = false
	if err != nil {
		if s.left {
			s.finish(false)
		}
		refuse(w, http.StatusUnprocessableEntity, err)
		return
	}
	s.finish(true)
	fmt.Fprintln(w, "ok")
}

// refuse tells the page why, in words it shows.
func refuse(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}
