package ui

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
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

// Server serves one page, on this machine only, until it gives a file back.
// A setting can name a command, so what a page sends must come from this
// page and no other: the token in the URL is known only to the page, and
// Host and Origin are checked, which keeps out a site that renames itself
// to 127.0.0.1 in DNS (ADR-100).
type Server struct {
	Page []byte
	// Check says what is wrong with a file sent; nil takes it. The error
	// is shown on the page, which stays open to fix it.
	Check func(src []byte) error

	token string
	host  string

	mu    sync.Mutex
	saved bool
	done  chan []byte
	// loads counts the times the page was read, and grace is how
	// long a page that went away has to be read again.
	loads int
	grace time.Duration
}

// Serve shows the page, calling open with its URL, and returns the file
// sent from it, or nil when the page was closed without one. Canceling ctx
// stops it, and returns ctx's error.
func (s *Server) Serve(ctx context.Context, open func(url string)) ([]byte, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		l.Close()
		return nil, err
	}
	s.token, s.host, s.done = hex.EncodeToString(b), l.Addr().String(), make(chan []byte, 1)
	if s.grace == 0 {
		s.grace = closeGrace
	}
	srv := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(l) }()
	open("http://" + s.host + "/" + s.token + "/")

	var src []byte
	select {
	case src = <-s.done:
	case <-ctx.Done():
		err = ctx.Err()
	}
	// Lets the answer to the page finish.
	stop, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = srv.Shutdown(stop)
	return src, err
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
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; img-src data:")
		s.mu.Lock()
		s.loads++
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

// finish ends Serve with src, once.
func (s *Server) finish(src []byte) {
	if !s.saved {
		s.saved = true
		s.done <- src
	}
}

// closePage ends it with nothing, as the page's Close asks.
func (s *Server) closePage(w http.ResponseWriter, r *http.Request) {
	if !s.ownOrigin(w, r) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.finish(nil)
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
			s.finish(nil)
		}
	})
}

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
	defer s.mu.Unlock()
	if s.saved {
		http.Error(w, "already saved", http.StatusConflict)
		return
	}
	if err := s.Check(src); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	s.finish(src)
	fmt.Fprintln(w, "ok")
}
