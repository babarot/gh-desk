package ui

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// fakeTerminal draws what it is given, and notes what it is told.
type fakeTerminal struct {
	mu      sync.Mutex
	draw    []string
	typed   []string
	applied []string
	refuse  error
	// wait keeps Run going until ctx ends, as gh-desk does until it quits.
	wait bool
}

func (f *fakeTerminal) Run(ctx context.Context, out io.Writer) error {
	for _, d := range f.draw {
		if _, err := out.Write([]byte(d)); err != nil {
			return err
		}
	}
	if f.wait {
		<-ctx.Done()
	}
	return nil
}

func (f *fakeTerminal) Input(p []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.typed = append(f.typed, string(p))
}

func (f *fakeTerminal) Apply(src []byte) error {
	if f.refuse != nil {
		return f.refuse
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applied = append(f.applied, string(src))
	return nil
}

func TestOnlyThePageTypesInTheTerminal(t *testing.T) {
	const host, origin = "127.0.0.1:1", "http://127.0.0.1:1"
	for _, tt := range []struct {
		name, method, url, host, origin string
		want                            int
	}{
		{"typing", http.MethodPost, "/secret/tty/in", host, origin, http.StatusNoContent},
		{"applying", http.MethodPost, "/secret/tty/apply", host, origin, http.StatusNoContent},
		{"typing from another site", http.MethodPost, "/secret/tty/in", host, "https://evil.example", http.StatusForbidden},
		{"typing with no origin", http.MethodPost, "/secret/tty/in", host, "", http.StatusForbidden},
		{"applying from another site", http.MethodPost, "/secret/tty/apply", host, "https://evil.example", http.StatusForbidden},
		{"typing with no token", http.MethodPost, "/tty/in", host, origin, http.StatusNotFound},
		{"reading under another name", http.MethodGet, "/secret/tty", "evil.example:1", "", http.StatusForbidden},
		{"reading with a wrong token", http.MethodGet, "/wrong/tty", host, "", http.StatusNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := testServer(func([]byte) error { return nil })
			term := &fakeTerminal{}
			s.Terminal = term
			if w := ask(s, tt.method, tt.url, tt.host, tt.origin, "j"); w.Code != tt.want {
				t.Errorf("code %d, want %d", w.Code, tt.want)
			}
			if tt.want != http.StatusNoContent && (len(term.typed) != 0 || len(term.applied) != 0) {
				t.Errorf("it reached gh-desk: %q %q", term.typed, term.applied)
			}
		})
	}
}

func TestNoTerminalNoRoutes(t *testing.T) {
	s := testServer(func([]byte) error { return nil })
	for _, path := range []string{"tty", "tty/in", "tty/apply"} {
		method := http.MethodPost
		if path == "tty" {
			method = http.MethodGet
		}
		if w := ask(s, method, "/secret/"+path, "127.0.0.1:1", "http://127.0.0.1:1", "j"); w.Code != http.StatusNotFound {
			t.Errorf("%s: code %d with no terminal", path, w.Code)
		}
	}
}

func TestTheTerminalStreamsWhatGhDeskDraws(t *testing.T) {
	s := testServer(func([]byte) error { return nil })
	draw := []string{"\x1b[2Jdesk", "\r\n review\n"}
	s.Terminal = &fakeTerminal{draw: draw}
	w := ask(s, http.MethodGet, "/secret/tty", "127.0.0.1:1", "", "")
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("code %d, %q", w.Code, w.Header().Get("Content-Type"))
	}
	var got []string
	ended := false
	for ev := range strings.SplitSeq(strings.TrimSpace(w.Body.String()), "\n\n") {
		if strings.HasPrefix(ev, "event: end") {
			ended = true
			continue
		}
		b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(ev, "data: "))
		if err != nil {
			t.Fatalf("an event is not base64: %q", ev)
		}
		got = append(got, string(b))
	}
	if strings.Join(got, "") != strings.Join(draw, "") {
		t.Errorf("drawn %q, want %q", got, draw)
	}
	if !ended {
		t.Error("the page is not told gh-desk quit")
	}
}

func TestTheTerminalStopsWithTheServer(t *testing.T) {
	s := testServer(func([]byte) error { return nil })
	s.Terminal = &fakeTerminal{wait: true}
	s.live, s.endLive = context.WithCancel(context.Background())
	done := make(chan *strings.Builder)
	go func() {
		w := ask(s, http.MethodGet, "/secret/tty", "127.0.0.1:1", "", "")
		b := &strings.Builder{}
		b.WriteString(w.Body.String())
		done <- b
	}()
	s.endLive()
	if b := <-done; strings.Contains(b.String(), "event: end") {
		t.Error("a stream the server ended says gh-desk quit")
	}
}

func TestApplyingAFileThatDoesNotRead(t *testing.T) {
	s := testServer(func([]byte) error { return nil })
	s.Terminal = &fakeTerminal{refuse: errors.New("theme.dark: no such theme")}
	w := ask(s, http.MethodPost, "/secret/tty/apply", "127.0.0.1:1", "http://127.0.0.1:1", "x")
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "no such theme") {
		t.Errorf("code %d, %s", w.Code, w.Body.String())
	}
}

func TestThePageHasATerminalOnlyWhenLive(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   Input
		want bool
	}{
		{"the command", Input{Live: true}, true},
		{"the command, drawn again", Input{}, false},
		{"the site", Input{Live: true, Playground: true}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			page, err := Page(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			s := string(page)
			if got := strings.Contains(s, "xterm.js 5.5.0"); got != tt.want {
				t.Errorf("xterm.js in the page: %v, want %v", got, tt.want)
			}
			if got := strings.Contains(s, "const LIVE=true"); got != tt.want {
				t.Errorf("LIVE: %v, want %v", got, tt.want)
			}
		})
	}
}
