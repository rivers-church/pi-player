package piplayer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// TestRunShutsDownOnContextCancel checks the server stops cleanly on SIGTERM
// instead of being killed mid-request. Start used to call log.Fatalf from
// library code and treat the clean-shutdown sentinel as a fatal error.
func TestRunShutsDownOnContextCancel(t *testing.T) {
	p := &Player{
		api:         &APIHandler{},
		conf:        &Config{Mount: mount{Dir: t.TempDir()}},
		playlist:    &Playlist{},
		ConnViewer:  NewConnWS(),
		ConnControl: NewConnWS(),
	}
	p.Server = NewServer(p, "127.0.0.1:0")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, p) }()

	// Give the listener a moment to come up, then ask it to stop.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v, want nil on a clean shutdown", err)
		}
	case <-time.After(shutdownTimeout + time.Second):
		t.Fatal("Run did not return after its context was cancelled")
	}
}

// TestServerHasTimeouts guards the read timeouts against being dropped again:
// without them one stalled connection ties up a goroutine indefinitely.
func TestServerHasTimeouts(t *testing.T) {
	p := &Player{
		api:         &APIHandler{},
		conf:        &Config{},
		playlist:    &Playlist{},
		ConnViewer:  NewConnWS(),
		ConnControl: NewConnWS(),
	}
	server := NewServer(p, ":8080")

	if server.ReadHeaderTimeout == 0 {
		t.Error("ReadHeaderTimeout is unset")
	}
	if server.ReadTimeout == 0 {
		t.Error("ReadTimeout is unset")
	}
	if server.IdleTimeout == 0 {
		t.Error("IdleTimeout is unset")
	}
}

// TestViewerURLFollowsServerPort: the kiosk browser used to be pointed at a
// hardcoded :8080 regardless of the -addr the server was given.
func TestViewerURLFollowsServerPort(t *testing.T) {
	p := &Player{conf: &Config{}}
	p.Server = &http.Server{Addr: ":9090"}

	if got, want := p.viewerURL(), "http://localhost:9090/viewer"; got != want {
		t.Errorf("viewerURL() = %q, want %q", got, want)
	}
}

// TestRouteAuth pins down which routes need a session. /api used to let any
// host on the network drive playback and list the media directory, and
// /ws/viewer let anyone kick the display off its socket.
func TestRouteAuth(t *testing.T) {
	mediaDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(mediaDir, "a.txt"), []byte("media"), 0o644); err != nil {
		t.Fatalf("writing fixture failed: %v", err)
	}

	p := &Player{
		api:         testAPIHandler(t, map[string]string{"control.html": "control", "viewer.html": "viewer", "settings.html": "settings"}),
		conf:        &Config{Mount: mount{Dir: mediaDir}},
		playlist:    &Playlist{},
		store:       newSessionStore(testSessionKey),
		ConnViewer:  NewConnWS(),
		ConnControl: NewConnWS(),
	}
	mux := setupRoutes(p)
	cookie := authenticatedCookie(t, p)

	const lan = "192.168.1.50:40000"
	const local = "127.0.0.1:40000"

	cases := []struct {
		name       string
		method     string
		path       string
		remoteAddr string
		cookie     bool
		wantAllow  bool
	}{
		{"api from the network without a session", http.MethodPost, "/api", lan, false, false},
		{"api with a session", http.MethodPost, "/api", lan, true, true},
		{"dircheck from the network without a session", http.MethodGet, "/api/dircheck", lan, false, false},
		{"control from the network without a session", http.MethodGet, "/control", lan, false, false},
		{"settings from the network without a session", http.MethodGet, "/settings", lan, false, false},
		{"media from the network without a session", http.MethodGet, "/content/a.txt", lan, false, false},
		{"media from the kiosk browser", http.MethodGet, "/content/a.txt", local, false, true},
		{"media from the network with a session", http.MethodGet, "/content/a.txt", lan, true, true},
		{"viewer from the kiosk browser", http.MethodGet, "/viewer", local, false, true},
		{"viewer from the network without a session", http.MethodGet, "/viewer", lan, false, false},
		{"assets are always served", http.MethodGet, "/assets/control.html", lan, false, true},
		{"login page is always served", http.MethodGet, "/login", lan, false, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			request := httptest.NewRequest(c.method, c.path, strings.NewReader(`{"component":"playlist","method":"getCurrent"}`))
			request.Header.Set("Content-Type", "application/json")
			request.RemoteAddr = c.remoteAddr
			if c.cookie {
				request.AddCookie(cookie)
			}
			recorder := httptest.NewRecorder()

			mux.ServeHTTP(recorder, request)

			refused := recorder.Code == http.StatusUnauthorized ||
				(recorder.Code == http.StatusFound && recorder.Header().Get("Location") == "/login")
			if c.wantAllow && refused {
				t.Errorf("%s %s was refused with status %d, want it allowed", c.method, c.path, recorder.Code)
			}
			if !c.wantAllow && !refused {
				t.Errorf("%s %s was allowed with status %d, want it refused", c.method, c.path, recorder.Code)
			}
		})
	}
}

func TestIsLoopback(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:40000":     true,
		"[::1]:40000":         true,
		"192.168.1.50:40000":  true,
		"10.0.0.1:80":         true,
		"127.0.0.1":           true,
		"not-an-address":      false,
		"999.999.999.999:123": false,
	}
	for addr, loopback := range cases {
		want := loopback && (addr == "127.0.0.1:40000" || addr == "[::1]:40000" || addr == "127.0.0.1")
		if got := isLoopback(addr); got != want {
			t.Errorf("isLoopback(%q) = %v, want %v", addr, got, want)
		}
	}
}
