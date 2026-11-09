package ui

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"sync"
)

// maxInput is the most one request of what is typed carries: keys, or a
// paste into a text box.
const maxInput = 64 << 10

// Terminal is gh-desk itself, run on the page in place of a drawing of it,
// against the lists last read: it reads nothing from GitHub and writes
// nothing (ADR-109). The caller makes it; the page's terminal is one at a
// time.
type Terminal interface {
	// Run runs gh-desk on the file last applied, drawing on out, until ctx
	// ends or gh-desk quits. A run started ends the one before it.
	Run(ctx context.Context, out io.Writer) error
	// Input is what is typed in the page's terminal.
	Input(p []byte)
	// Apply gives gh-desk the file src: the run going takes it, and the
	// next one starts with it. Its error means the file does not read yet,
	// and nothing changes.
	Apply(src []byte) error
}

// events writes what gh-desk draws as server-sent events. An event is a
// line of text, so the bytes go in base64.
type events struct {
	mu sync.Mutex
	w  http.ResponseWriter
	rc *http.ResponseController
}

func (e *events) Write(p []byte) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, err := io.WriteString(e.w, "data: "+base64.StdEncoding.EncodeToString(p)+"\n\n"); err != nil {
		return 0, err
	}
	return len(p), e.rc.Flush()
}

// end tells the page gh-desk quit.
func (e *events) end() {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, _ = io.WriteString(e.w, "event: end\ndata: \n\n")
	_ = e.rc.Flush()
}

// tty streams gh-desk to the page until the page goes, the server stops or
// gh-desk quits. A GET from the page carries no Origin, as reading the page
// does not; the token and Host keep it the page's own.
func (s *Server) tty(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	if s.live != nil {
		stop := context.AfterFunc(s.live, cancel)
		defer stop()
	}
	e := &events{w: w, rc: http.NewResponseController(w)}
	w.WriteHeader(http.StatusOK)
	_ = e.rc.Flush()
	if err := s.Terminal.Run(ctx, e); err != nil {
		_, _ = e.Write([]byte("\r\ngh-desk: " + err.Error() + "\r\n"))
	}
	if ctx.Err() == nil {
		e.end()
	}
}

// ttyInput gives gh-desk what is typed.
func (s *Server) ttyInput(w http.ResponseWriter, r *http.Request) {
	if !s.ownOrigin(w, r) {
		return
	}
	p, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxInput))
	if err != nil {
		http.Error(w, "too large", http.StatusRequestEntityTooLarge)
		return
	}
	s.Terminal.Input(p)
	w.WriteHeader(http.StatusNoContent)
}

// ttyApply gives gh-desk the file as it stands on the page.
func (s *Server) ttyApply(w http.ResponseWriter, r *http.Request) {
	if !s.ownOrigin(w, r) {
		return
	}
	src, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxFile))
	if err != nil {
		http.Error(w, "too large", http.StatusRequestEntityTooLarge)
		return
	}
	if err := s.Terminal.Apply(src); err != nil {
		refuse(w, http.StatusUnprocessableEntity, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
