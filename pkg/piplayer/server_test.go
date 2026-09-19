package piplayer

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestContentServedFromCurrentDir verifies that /content/ resolves the media
// directory from the config on each request. This is what lets a settings-page
// media-dir change take effect immediately, without recreating the server —
// the reason restart() could be removed.
func TestContentServedFromCurrentDir(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirA, "a.txt"), []byte("from A"), 0o644); err != nil {
		t.Fatalf("failed to write fixture in dirA: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirB, "b.txt"), []byte("from B"), 0o644); err != nil {
		t.Fatalf("failed to write fixture in dirB: %v", err)
	}

	p := &Player{conf: &Config{Mount: mount{Dir: dirA}}}
	handler := http.HandlerFunc(contentHandler(p))

	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		handler.ServeHTTP(rec, req)
		return rec
	}

	// Initially serving from dirA.
	if rec := get("/content/a.txt"); rec.Code != http.StatusOK || rec.Body.String() != "from A" {
		t.Errorf("dirA: got status %d body %q, want 200 %q", rec.Code, rec.Body.String(), "from A")
	}

	// Change the configured media dir, as SettingsHandler would.
	p.conf.Mount.Dir = dirB

	// The same handler instance now serves from dirB without being rebuilt.
	if rec := get("/content/b.txt"); rec.Code != http.StatusOK || rec.Body.String() != "from B" {
		t.Errorf("dirB: got status %d body %q, want 200 %q", rec.Code, rec.Body.String(), "from B")
	}

	// A file that only existed in the old dir is no longer served.
	if rec := get("/content/a.txt"); rec.Code != http.StatusNotFound {
		t.Errorf("stale file: got status %d, want 404", rec.Code)
	}
}

// TestRouteMethods checks that the mux itself rejects the wrong method, so the
// handlers don't have to. Logout in particular must not be reachable with a
// GET, or any page can log the operator out with an <img> tag.
func TestRouteMethods(t *testing.T) {
	p := &Player{
		api:         &APIHandler{},
		conf:        &Config{Mount: mount{Dir: t.TempDir()}},
		playlist:    &Playlist{},
		ConnViewer:  NewConnWS(),
		ConnControl: NewConnWS(),
	}
	mux := setupRoutes(p)

	cases := []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/logout", http.StatusMethodNotAllowed},
		{http.MethodGet, "/api", http.StatusMethodNotAllowed},
		{http.MethodPut, "/api", http.StatusMethodNotAllowed},
		{http.MethodPost, "/control", http.StatusMethodNotAllowed},
		{http.MethodPost, "/viewer", http.StatusMethodNotAllowed},
		{http.MethodPost, "/api/dircheck", http.StatusMethodNotAllowed},
		{http.MethodGet, "/no/such/page", http.StatusNotFound},
	}

	for _, c := range cases {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
		if rec.Code != c.want {
			t.Errorf("%s %s: got status %d, want %d", c.method, c.path, rec.Code, c.want)
		}
	}
}
