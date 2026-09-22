package piplayer

import (
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

//go:embed assets templates
var realFiles embed.FS

// errMissingDir stands in for the error the playlist returns when the media
// directory has gone away.
var errMissingDir = errors.New("media directory is not readable")

// TestLogoutClearsTheSession is the security half of login: the cookie it
// hands back must stop working.
func TestLogoutClearsTheSession(t *testing.T) {
	p := newTestPlayer(t)
	cookie := authenticatedCookie(t, p)

	// The cookie authenticates before logging out.
	check := httptest.NewRequest(http.MethodGet, "http://piplayer.local/control", nil)
	check.AddCookie(cookie)
	if _, loggedIn, _ := p.checkLogin(httptest.NewRecorder(), check); !loggedIn {
		t.Fatal("the test cookie does not authenticate to begin with")
	}

	request := httptest.NewRequest(http.MethodPost, "http://piplayer.local/logout", nil)
	request.AddCookie(cookie)
	recorder := httptest.NewRecorder()

	p.handleLogout(recorder, request)

	response := recorder.Result()
	defer response.Body.Close()
	if response.StatusCode != http.StatusFound || response.Header.Get("Location") != "/login" {
		t.Errorf("logout returned %d to %q, want a redirect to /login", response.StatusCode, response.Header.Get("Location"))
	}

	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("logout set %d cookies, want 1 expiring one", len(cookies))
	}
	if cookies[0].MaxAge >= 0 {
		t.Errorf("the logout cookie has MaxAge %d, want it negative so the browser drops it", cookies[0].MaxAge)
	}

	// The replacement cookie must not authenticate.
	after := httptest.NewRequest(http.MethodGet, "http://piplayer.local/control", nil)
	after.AddCookie(cookies[0])
	if _, loggedIn, _ := p.checkLogin(httptest.NewRecorder(), after); loggedIn {
		t.Error("the cookie handed back by logout still authenticates")
	}
}

// TestLogoutWithoutSession checks logging out when you already are doesn't
// blow up - the error page's links can get there.
func TestLogoutWithoutSession(t *testing.T) {
	p := newTestPlayer(t)
	recorder := httptest.NewRecorder()

	p.handleLogout(recorder, httptest.NewRequest(http.MethodPost, "http://piplayer.local/logout", nil))

	if recorder.Code != http.StatusFound {
		t.Errorf("logout without a session returned %d, want a redirect", recorder.Code)
	}
}

func TestHandleDirCheck(t *testing.T) {
	present := t.TempDir()
	p := newTestPlayer(t, withMediaDir(present))

	recorder := httptest.NewRecorder()
	p.handleDirCheck(recorder, httptest.NewRequest(http.MethodGet, "/api/dircheck", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d, want %d", recorder.Code, http.StatusOK)
	}
	if got := strings.TrimSpace(recorder.Body.String()); got != `{"ok":true}` {
		t.Errorf("got %s, want {\"ok\":true}", got)
	}

	// The same check with the directory gone is what the error page polls on.
	p.conf.setMediaDir(present + "-does-not-exist")
	recorder = httptest.NewRecorder()
	p.handleDirCheck(recorder, httptest.NewRequest(http.MethodGet, "/api/dircheck", nil))

	if got := strings.TrimSpace(recorder.Body.String()); got != `{"ok":false}` {
		t.Errorf("got %s for a missing directory, want {\"ok\":false}", got)
	}
}

func TestHandleHomeRedirects(t *testing.T) {
	p := newTestPlayer(t)
	cookie := authenticatedCookie(t, p)

	cases := []struct {
		name   string
		cookie bool
		want   string
	}{
		{"logged out goes to the login page", false, "/login"},
		{"logged in goes to the controls", true, "/control"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://piplayer.local/", nil)
			if c.cookie {
				request.AddCookie(cookie)
			}
			recorder := httptest.NewRecorder()

			p.handleHome(recorder, request)

			if recorder.Code != http.StatusFound {
				t.Fatalf("status %d, want %d", recorder.Code, http.StatusFound)
			}
			if got := recorder.Header().Get("Location"); got != c.want {
				t.Errorf("redirected to %q, want %q", got, c.want)
			}
		})
	}
}

// TestRenderErrorPage covers the page the kiosk shows when the media
// directory is unreadable, which is the most common real-world failure.
func TestRenderErrorPage(t *testing.T) {
	p := newTestPlayer(t, withTemplates(map[string]string{
		"error.html": `<body data-redirect="{{.Redirect}}">{{.Error}} in {{.Dir}} on {{.Port}}</body>`,
	}), withMediaDir("/gone"))
	p.Server = &http.Server{Addr: ":8080"}

	recorder := httptest.NewRecorder()
	p.renderErrorPage(recorder, errMissingDir, "/viewer")

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("status %d, want %d - a failure page reported as success is worse than useless", recorder.Code, http.StatusInternalServerError)
	}
	body := recorder.Body.String()
	for _, want := range []string{errMissingDir.Error(), "/gone", "8080", `data-redirect="/viewer"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the error page does not mention %q: %s", want, body)
		}
	}
}

// TestNewAPIHandlerParsesEmbedded builds the handler from the real templates
// and assets, so a renamed or broken one fails here rather than at runtime.
// main.go embeds them from the repo root, so the test has to present them at
// the paths NewAPIHandler looks under.
func TestNewAPIHandlerParsesEmbedded(t *testing.T) {
	mode := ""
	api, err := NewAPIHandler(&mode, rerootEmbedded(t), rerootEmbedded(t))
	if err != nil {
		t.Fatalf("building the API handler from the embedded files failed: %v", err)
	}

	for _, name := range []string{"control.html", "viewer.html", "login.html", "settings.html", "error.html"} {
		if api.templates.Lookup(name) == nil {
			t.Errorf("template %q is missing from the parsed set", name)
		}
	}
	if _, err := api.statAssets.Open("js/control.js"); err != nil {
		t.Errorf("the assets filesystem does not serve js/control.js: %v", err)
	}
}

// rerootEmbedded presents the package's own embedded files at the paths
// main.go embeds them under, so the test can build the handler the real way.
func rerootEmbedded(t *testing.T) fstest.MapFS {
	t.Helper()

	rerooted := fstest.MapFS{}
	for _, dir := range []string{"assets", "templates"} {
		err := fs.WalkDir(realFiles, dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			data, err := fs.ReadFile(realFiles, path)
			if err != nil {
				return err
			}
			rerooted["pkg/piplayer/"+path] = &fstest.MapFile{Data: data}
			return nil
		})
		if err != nil {
			t.Fatalf("copying the embedded %s failed: %v", dir, err)
		}
	}

	return rerooted
}
