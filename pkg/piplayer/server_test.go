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
	handler := http.HandlerFunc(etagWrapper(p))

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
