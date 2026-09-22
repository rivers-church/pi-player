package piplayer

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

// testSessionKey signs cookies in tests. Real deployments generate their own.
var testSessionKey = []byte("test-session-key-not-used-in-production")

// playerOpt adjusts the test player built by newTestPlayer.
type playerOpt func(*testing.T, *Player)

// newTestPlayer builds a Player with every field populated, so a handler that
// starts touching a new one doesn't nil-panic a test that didn't know about it.
func newTestPlayer(t *testing.T, opts ...playerOpt) *Player {
	t.Helper()

	p := &Player{
		api:         testAPIHandler(t, nil),
		conf:        &Config{},
		playlist:    &Playlist{},
		store:       newSessionStore(testSessionKey),
		loginLimit:  newLoginLimiter(),
		ConnViewer:  newConnWS(),
		ConnControl: newConnWS(),
	}
	for _, opt := range opts {
		opt(t, p)
	}
	return p
}

// withTemplates gives the player stub templates, keyed by file name.
func withTemplates(files map[string]string) playerOpt {
	return func(t *testing.T, p *Player) {
		t.Helper()
		p.api = testAPIHandler(t, files)
	}
}

// withMediaDir points the player at a media directory.
func withMediaDir(dir string) playerOpt {
	return func(t *testing.T, p *Player) {
		t.Helper()
		p.conf.setMediaDir(dir)
	}
}

// withSessionKey signs this player's cookies with its own key.
func withSessionKey(key []byte) playerOpt {
	return func(t *testing.T, p *Player) {
		t.Helper()
		p.store = newSessionStore(key)
	}
}

// testAPIHandler builds an APIHandler whose templates are the given stubs,
// keyed by file name.
func testAPIHandler(t *testing.T, files map[string]string) *APIHandler {
	t.Helper()

	fsys := fstest.MapFS{}
	for name, body := range files {
		fsys[name] = &fstest.MapFile{Data: []byte(body)}
	}
	// ParseFS fails on an empty pattern match, and plenty of tests never
	// render a page.
	if len(fsys) == 0 {
		fsys["blank.html"] = &fstest.MapFile{Data: []byte("")}
	}

	templates, err := template.ParseFS(fsys, "*.html")
	if err != nil {
		t.Fatalf("parsing test templates failed: %v", err)
	}
	// statAssets is the same stub filesystem: the asset route needs something
	// non-nil to serve from.
	return &APIHandler{statAssets: fsys, templates: templates}
}

// authenticatedCookie returns a cookie for a logged-in session on p.
func authenticatedCookie(t *testing.T, p *Player) *http.Cookie {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "http://piplayer.local/", nil)
	session, err := p.store.Get(request, "piplayer-session")
	if err != nil {
		t.Fatalf("creating authenticated session failed: %v", err)
	}
	session.Values["authenticated"] = "test"
	recorder := httptest.NewRecorder()
	if err := session.Save(request, recorder); err != nil {
		t.Fatalf("saving authenticated session failed: %v", err)
	}
	return recorder.Result().Cookies()[0]
}

// writeFiles creates each named file (empty unless content given) in dir.
func writeFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte{}, 0o644); err != nil {
			t.Fatalf("failed to write fixture %q: %v", name, err)
		}
	}
}

// eventually polls until want returns true, or fails the test. Tests wait on a
// condition rather than sleeping for a guessed duration, which is both faster
// and steadier on a loaded machine.
func eventually(t *testing.T, timeout time.Duration, what string, want func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if want() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out after %s waiting for %s", timeout, what)
}
