package configui

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testServer(check func([]byte) error) *Server {
	return &Server{Page: []byte("the page"), Check: check, token: "secret", host: "127.0.0.1:1", done: make(chan []byte, 1)}
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
	s := testServer(func([]byte) error {
		if wrong {
			return errors.New("not yet")
		}
		return nil
	})
	if w := ask(s, http.MethodPost, "/secret/save", host, origin, "one"); w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "not yet") {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	wrong = false
	if w := ask(s, http.MethodPost, "/secret/save", host, origin, "two"); w.Code != http.StatusOK {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if got := <-s.done; string(got) != "two" {
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
	if got := <-s.done; got != nil {
		t.Errorf("closed with %q", got)
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
	case got := <-s.done:
		if got != nil {
			t.Errorf("ended with %q", got)
		}
	case <-time.After(time.Second):
		t.Error("it did not end")
	}
}

func TestServeGivesBackWhatIsSaved(t *testing.T) {
	s := &Server{Page: []byte("the page"), Check: func([]byte) error { return nil }}
	type result struct {
		src []byte
		err error
	}
	done := make(chan result, 1)
	urls := make(chan string, 1)
	go func() {
		src, err := s.Serve(t.Context(), func(u string) { urls <- u })
		done <- result{src, err}
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
	if r := <-done; r.err != nil || string(r.src) != "the file" {
		t.Errorf("%q, %v", r.src, r.err)
	}
}
