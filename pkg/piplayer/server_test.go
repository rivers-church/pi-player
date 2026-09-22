package piplayer

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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

	p := newTestPlayer(t, withMediaDir(dirA))
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
	p.conf.setMediaDir(dirB)

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
	p := newTestPlayer(t, withMediaDir(t.TempDir()))
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
	p := newTestPlayer(t, withMediaDir(t.TempDir()))
	p.Server = NewServer(p, freePort(t))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, p) }()

	// Wait for the listener to actually accept before asking it to stop, so a
	// slow start doesn't turn into a shutdown of something not yet serving.
	eventually(t, 2*time.Second, "the server to start listening", func() bool {
		conn, err := net.Dial("tcp", p.Server.Addr)
		if err != nil {
			return false
		}
		conn.Close()
		return true
	})
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
	p := newTestPlayer(t)
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
	p := newTestPlayer(t)
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

	p := newTestPlayer(t,
		withTemplates(map[string]string{"control.html": "control", "viewer.html": "viewer", "settings.html": "settings"}),
		withMediaDir(mediaDir))
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

// TestCrossOriginPostRefused covers the case SameSite=Lax is meant to stop
// anyway: a form on another site posting to the settings or login page.
func TestCrossOriginPostRefused(t *testing.T) {
	p := newTestPlayer(t,
		withTemplates(map[string]string{"control.html": "control"}),
		withMediaDir(t.TempDir()))
	handler := NewServer(p, ":8080").Handler
	cookie := authenticatedCookie(t, p)

	cases := []struct {
		name       string
		origin     string
		wantStatus int
	}{
		{"from another site", "http://evil.example.com", http.StatusForbidden},
		{"from the player itself", "http://piplayer.local", 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			form := url.Values{"location": {"Somewhere"}}.Encode()
			request := httptest.NewRequest(http.MethodPost, "http://piplayer.local/settings", strings.NewReader(form))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("Origin", c.origin)
			request.AddCookie(cookie)
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, request)

			if c.wantStatus != 0 && recorder.Code != c.wantStatus {
				t.Errorf("got status %d, want %d", recorder.Code, c.wantStatus)
			}
			if c.wantStatus == 0 && recorder.Code == http.StatusForbidden {
				t.Error("a same-origin POST was refused")
			}
		})
	}
}

func TestSecurityHeaders(t *testing.T) {
	p := newTestPlayer(t, withTemplates(map[string]string{"login.html": "login"}))
	recorder := httptest.NewRecorder()

	NewServer(p, ":8080").Handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/login", nil))

	want := map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "same-origin",
		"Content-Security-Policy": contentSecurityPolicy,
	}
	for header, value := range want {
		if got := recorder.Header().Get(header); got != value {
			t.Errorf("%s = %q, want %q", header, got, value)
		}
	}

	// The policy is only worth having if it keeps scripts to this origin.
	if !strings.Contains(contentSecurityPolicy, "script-src 'self';") {
		t.Errorf("the content security policy does not restrict scripts to this origin: %q", contentSecurityPolicy)
	}
}

// TestKioskPathWorksWithoutSession walks the requests the kiosk Chromium makes
// on startup, from localhost with no cookie. The viewer page fetches its items
// over /api and the error page polls /api/dircheck, so guarding those with a
// session-only check leaves the display up but empty.
func TestKioskPathWorksWithoutSession(t *testing.T) {
	mediaDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(mediaDir, "clip.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatalf("writing fixture failed: %v", err)
	}

	p := newTestPlayer(t,
		withTemplates(map[string]string{"viewer.html": "viewer", "control.html": "control"}),
		withMediaDir(mediaDir))
	mux := setupRoutes(p)

	kiosk := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", "http://"+request.Host)
		}
		request.RemoteAddr = "127.0.0.1:41000"
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		return recorder
	}

	if rec := kiosk(http.MethodGet, "/viewer", ""); rec.Code != http.StatusOK {
		t.Errorf("the kiosk could not load /viewer: status %d", rec.Code)
	}

	rec := kiosk(http.MethodPost, "/api", `{"component":"playlist","method":"getItems"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("the kiosk could not fetch its playlist: status %d, body %q", rec.Code, rec.Body.String())
	}
	var res resMessage
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("decoding the playlist response failed: %v", err)
	}
	if !res.Success {
		t.Errorf("getItems reported failure: %+v", res)
	}
	items, ok := res.Message.([]any)
	if !ok || len(items) != 1 {
		t.Errorf("the kiosk got %v, want the one item in the media directory", res.Message)
	}

	if rec := kiosk(http.MethodGet, "/api/dircheck", ""); rec.Code != http.StatusOK {
		t.Errorf("the error page could not poll /api/dircheck: status %d", rec.Code)
	}

	// The same requests from elsewhere on the network must still be refused.
	lan := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", "http://"+request.Host)
		}
		request.RemoteAddr = "192.168.1.50:41000"
		recorder := httptest.NewRecorder()
		mux.ServeHTTP(recorder, request)
		return recorder
	}

	if rec := lan(http.MethodPost, "/api", `{"component":"playlist","method":"getItems"}`); rec.Code != http.StatusUnauthorized {
		t.Errorf("/api from the network returned %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if rec := lan(http.MethodGet, "/api/dircheck", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("/api/dircheck from the network returned %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if rec := lan(http.MethodGet, "/viewer", ""); rec.Code != http.StatusFound {
		t.Errorf("/viewer from the network returned %d, want a redirect to the login page", rec.Code)
	}
}

// freePort returns a loopback address that nothing is listening on, so a test
// can wait for the server to come up by dialing it. Asking the server for
// ":0" would leave the test with no port to dial.
func freePort(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("finding a free port failed: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("releasing the port failed: %v", err)
	}
	return addr
}

// TestDebugLoggingFollowsSettings checks the settings page moves the log level
// at runtime. The debug flag used to be stored twice - in the config, which the
// settings page changed, and on the API handler, which snapshotted it at
// startup - so half the log sites ignored the toggle.
func TestDebugLoggingFollowsSettings(t *testing.T) {
	t.Cleanup(func() { SetDebugLogging(false) })

	SetDebugLogging(false)
	if logger.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("debug logging is on with the setting off")
	}

	SetDebugLogging(true)
	if !logger.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("debug logging is off with the setting on")
	}
}
