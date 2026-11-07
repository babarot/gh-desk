package ui

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testServer(check func([]byte) error) *Server {
	return &Server{Page: []byte("the page"), Save: func(_ context.Context, src []byte) error { return check(src) },
		token: "secret", host: "127.0.0.1:1", ctx: context.Background(), done: make(chan bool, 1)}
}

func ask(s *Server, method, url, host, origin, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, url, strings.NewReader(body))
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	return w
}

func TestOnlyThePageGetsIn(t *testing.T) {
	const host, origin = "127.0.0.1:1", "http://127.0.0.1:1"
	for _, tt := range []struct {
		name, method, url, host, origin string
		want                            int
	}{
		{"the page", http.MethodGet, "/secret/", host, "", http.StatusOK},
		{"no token", http.MethodGet, "/", host, "", http.StatusNotFound},
		{"a wrong token", http.MethodGet, "/wrong/", host, "", http.StatusNotFound},
		{"another name for this host", http.MethodGet, "/secret/", "evil.example:1", "", http.StatusForbidden},
		{"saving with no token", http.MethodPost, "/save", host, origin, http.StatusNotFound},
		{"saving from another site", http.MethodPost, "/secret/save", host, "https://evil.example", http.StatusForbidden},
		{"saving with no origin", http.MethodPost, "/secret/save", host, "", http.StatusForbidden},
		{"another path", http.MethodGet, "/secret/other", host, "", http.StatusNotFound},
		{"saving by get", http.MethodGet, "/secret/save", host, "", http.StatusNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := testServer(func([]byte) error { return nil })
			if w := ask(s, tt.method, tt.url, tt.host, tt.origin, "x"); w.Code != tt.want {
				t.Errorf("code %d, want %d", w.Code, tt.want)
			}
			if tt.want != http.StatusOK && len(s.done) != 0 {
				t.Error("it was saved")
			}
		})
	}
}

func TestSave(t *testing.T) {
	const host, origin = "127.0.0.1:1", "http://127.0.0.1:1"
	wrong := true
	var got string
	s := testServer(func(src []byte) error {
		if wrong {
			return errors.New("not yet")
		}
		got = string(src)
		return nil
	})
	if w := ask(s, http.MethodPost, "/secret/save", host, origin, "one"); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "not yet") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	wrong = false
	if w := ask(s, http.MethodPost, "/secret/save", host, origin, "two"); w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if !<-s.done || got != "two" {
		t.Errorf("saved %q", got)
	}
	if w := ask(s, http.MethodPost, "/secret/save", host, origin, "three"); w.Code != http.StatusConflict {
		t.Errorf("saved twice: %d", w.Code)
	}
	if w := ask(s, http.MethodPost, "/secret/save", host, origin, strings.Repeat("x", maxFile+1)); w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("too large: %d", w.Code)
	}
}

func TestClose(t *testing.T) {
	const host, origin = "127.0.0.1:1", "http://127.0.0.1:1"
	s := testServer(func([]byte) error { return nil })
	if w := ask(s, http.MethodPost, "/secret/close", host, "https://evil.example", ""); w.Code != http.StatusForbidden || len(s.done) != 0 {
		t.Fatalf("closed from another site: %d", w.Code)
	}
	if w := ask(s, http.MethodPost, "/wrong/close", host, origin, ""); w.Code != http.StatusNotFound {
		t.Fatalf("closed with a wrong token: %d", w.Code)
	}
	ask(s, http.MethodPost, "/secret/close", host, origin, "")
	if <-s.done {
		t.Error("closed as saved")
	}
}

func TestATabThatGoesEndsIt(t *testing.T) {
	const host, origin = "127.0.0.1:1", "http://127.0.0.1:1"
	s := testServer(func([]byte) error { return nil })
	s.grace = 20 * time.Millisecond
	ask(s, http.MethodGet, "/secret/", host, "", "")

	// A reload comes back in time.
	ask(s, http.MethodPost, "/secret/gone", host, origin, "")
	ask(s, http.MethodGet, "/secret/", host, "", "")
	time.Sleep(60 * time.Millisecond)
	if len(s.done) != 0 {
		t.Fatal("a reload ended it")
	}

	ask(s, http.MethodPost, "/secret/gone", host, origin, "")
	select {
	case saved := <-s.done:
		if saved {
			t.Error("ended as saved")
		}
	case <-time.After(time.Second):
		t.Error("it did not end")
	}
}

// The page may go while the terminal asks: the answer still counts, and a
// no ends the command, as nothing is left to edit.
func TestATabThatGoesDuringTheQuestion(t *testing.T) {
	const host, origin = "127.0.0.1:1", "http://127.0.0.1:1"
	for _, yes := range []bool{true, false} {
		answer := make(chan error)
		asking := make(chan struct{})
		s := testServer(func([]byte) error {
			close(asking)
			return <-answer
		})
		s.grace = time.Millisecond
		ask(s, http.MethodGet, "/secret/", host, "", "")
		saving := make(chan int)
		go func() { saving <- ask(s, http.MethodPost, "/secret/save", host, origin, "x").Code }()
		<-asking

		// No lock is held while it asks: a second save is refused at once.
		if w := ask(s, http.MethodPost, "/secret/save", host, origin, "y"); w.Code != http.StatusConflict {
			t.Errorf("a second save: %d", w.Code)
		}
		ask(s, http.MethodPost, "/secret/gone", host, origin, "")
		time.Sleep(30 * time.Millisecond)
		if len(s.done) != 0 {
			t.Fatal("it ended during the question")
		}
		if yes {
			answer <- nil
		} else {
			answer <- errors.New("no")
		}
		<-saving
		if got := <-s.done; got != yes {
			t.Errorf("yes %v: saved %v", yes, got)
		}
	}
}

func TestJudgeWritesNothing(t *testing.T) {
	const host, origin = "127.0.0.1:1", "http://127.0.0.1:1"
	s := testServer(func([]byte) error { t.Error("saved"); return nil })
	s.Judge = func(src []byte) ([]byte, error) {
		if string(src) == "wrong" {
			return nil, errors.New("does not read")
		}
		return []byte(`{"a":1}`), nil
	}
	if w := ask(s, http.MethodPost, "/secret/judge", host, origin, "ok"); w.Code != http.StatusOK || w.Body.String() != `{"a":1}` {
		t.Errorf("%d %s", w.Code, w.Body)
	}
	if w := ask(s, http.MethodPost, "/secret/judge", host, origin, "wrong"); w.Code != http.StatusUnprocessableEntity {
		t.Errorf("%d %s", w.Code, w.Body)
	}
	if w := ask(s, http.MethodPost, "/secret/judge", host, "https://evil.example", "ok"); w.Code != http.StatusForbidden {
		t.Errorf("from another site: %d", w.Code)
	}
	if w := ask(s, http.MethodPost, "/wrong/judge", host, origin, "ok"); w.Code != http.StatusNotFound {
		t.Errorf("with a wrong token: %d", w.Code)
	}
	if len(s.done) != 0 {
		t.Error("it ended")
	}
}

func TestATitleCannotLeaveItsScript(t *testing.T) {
	page, err := Page(Input{Lists: Lists{Sample: []PR{{Title: `</script><script>alert(1)</script>`}}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(page), "<script>alert(1)") {
		t.Error("the title ends the script")
	}
}

func TestThePageCannotBeFramed(t *testing.T) {
	s := testServer(func([]byte) error { return nil })
	w := ask(s, http.MethodGet, "/secret/", "127.0.0.1:1", "", "")
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Error(w.Header())
	}
}

func TestServeGivesBackWhatIsSaved(t *testing.T) {
	var file []byte
	s := &Server{Page: []byte("the page"), Save: func(_ context.Context, src []byte) error { file = src; return nil }}
	type result struct {
		saved bool
		err   error
	}
	done := make(chan result, 1)
	urls := make(chan string, 1)
	go func() {
		saved, err := s.Serve(t.Context(), func(u string) { urls <- u })
		done <- result{saved, err}
	}()
	u := <-urls
	res, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if string(b) != "the page" || !strings.HasPrefix(u, "http://127.0.0.1:") {
		t.Errorf("%s: %q", u, b)
	}
	req, _ := http.NewRequest(http.MethodPost, u+"save", strings.NewReader("the file"))
	req.Header.Set("Origin", "http://"+req.URL.Host)
	if res, err = http.DefaultClient.Do(req); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if r := <-done; r.err != nil || !r.saved || string(file) != "the file" {
		t.Errorf("%q, %v, %v", file, r.saved, r.err)
	}
}
