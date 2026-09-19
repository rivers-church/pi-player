package piplayer

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/17xande/configdir"
)

// emptyAssets stands in for the embedded asset FS. The logo copy is best-effort,
// so an empty FS is fine — configLoadFromPath just logs and moves on.
var emptyAssets embed.FS

func TestConfigLoadFirstRun(t *testing.T) {
	configPath := t.TempDir()
	mediaDir := filepath.Join(t.TempDir(), "media") // must not exist yet

	conf, err := configLoadFromPath(configPath, mediaDir, emptyAssets)
	if err != nil {
		t.Fatalf("configLoadFromPath() returned error: %v", err)
	}

	if conf.Location != "PiPlayer" {
		t.Errorf("Location = %q, want PiPlayer", conf.Location)
	}
	if conf.Login.Username != "admin" {
		t.Errorf("Login.Username = %q, want admin", conf.Login.Username)
	}
	if conf.Mount.Dir != mediaDir {
		t.Errorf("Mount.Dir = %q, want %q", conf.Mount.Dir, mediaDir)
	}

	// The media directory should have been created.
	if !exists(mediaDir) {
		t.Errorf("media dir %q was not created", mediaDir)
	}

	// config.json should have been written.
	if !exists(filepath.Join(configPath, "config.json")) {
		t.Error("config.json was not created on first run")
	}
}

func TestConfigLoadExisting(t *testing.T) {
	configPath := t.TempDir()
	mediaDir := t.TempDir() // already exists, so no first-run media setup

	existing := `{
		"Location": "Lobby",
		"Debug": false,
		"Login": {"Username": "operator", "Password": "hashed"},
		"Mount": {"URL": "/srv/media"}
	}`
	if err := os.WriteFile(filepath.Join(configPath, "config.json"), []byte(existing), 0o600); err != nil {
		t.Fatalf("failed to write existing config: %v", err)
	}

	conf, err := configLoadFromPath(configPath, mediaDir, emptyAssets)
	if err != nil {
		t.Fatalf("configLoadFromPath() returned error: %v", err)
	}

	if conf.Location != "Lobby" {
		t.Errorf("Location = %q, want Lobby", conf.Location)
	}
	if conf.Debug {
		t.Error("Debug = true, want false")
	}
	if conf.Login.Username != "operator" {
		t.Errorf("Login.Username = %q, want operator", conf.Login.Username)
	}
}

func TestConfigSaveRoundTrip(t *testing.T) {
	configPath := t.TempDir()
	mediaDir := t.TempDir()

	conf := &Config{
		Location: "Auditorium",
		Debug:    true,
		Login:    Login{Username: "alex", Password: "secret-hash"},
		Mount:    mount{URL: sURL{URL: &url.URL{Path: "/srv/media"}}, Dir: "/srv/media"},
	}

	if err := conf.saveToPath(configPath); err != nil {
		t.Fatalf("saveToPath() returned error: %v", err)
	}

	loaded, err := configLoadFromPath(configPath, mediaDir, emptyAssets)
	if err != nil {
		t.Fatalf("configLoadFromPath() returned error: %v", err)
	}

	if loaded.Location != conf.Location {
		t.Errorf("Location round-trip: got %q, want %q", loaded.Location, conf.Location)
	}
	if loaded.Debug != conf.Debug {
		t.Errorf("Debug round-trip: got %v, want %v", loaded.Debug, conf.Debug)
	}
	if loaded.Login != conf.Login {
		t.Errorf("Login round-trip: got %+v, want %+v", loaded.Login, conf.Login)
	}
}

// TestConfigSaveIsValidJSON guards the on-disk format stays parseable.
func TestConfigSaveIsValidJSON(t *testing.T) {
	configPath := t.TempDir()

	conf := &Config{
		Location: "Test",
		Mount:    mount{URL: sURL{URL: &url.URL{Path: "/srv/media"}}, Dir: "/srv/media"},
	}
	if err := conf.saveToPath(configPath); err != nil {
		t.Fatalf("saveToPath() returned error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(configPath, "config.json"))
	if err != nil {
		t.Fatalf("failed to read written config: %v", err)
	}

	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Errorf("written config.json is not valid JSON: %v", err)
	}
}

func TestSettingsHandlerSavesNewCredentials(t *testing.T) {
	// Point configdir at a temp dir so conf.Save() writes there instead of the
	// real user config, then check the new password survives a reload.
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	configdir.Refresh()
	t.Cleanup(configdir.Refresh)
	configPath := filepath.Join(configHome, "pi-player")
	if err := os.MkdirAll(configPath, 0755); err != nil {
		t.Fatalf("creating temp config dir failed: %v", err)
	}

	login, err := newLogin()
	if err != nil {
		t.Fatalf("creating default login failed: %v", err)
	}
	mediaDir := t.TempDir()
	conf := &Config{
		Location: "PiPlayer",
		Mount:    mount{URL: sURL{URL: &url.URL{Path: mediaDir}}, Dir: mediaDir},
		Login:    login,
	}
	p := &Player{conf: conf, api: &APIHandler{}}

	form := url.Values{
		"username": {"alex"},
		"password": {"a new password"},
	}.Encode()
	request := httptest.NewRequest(http.MethodPost, "http://piplayer.local/settings", strings.NewReader(form))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	sessionRecorder := httptest.NewRecorder()
	session, err := store.Get(request, "piplayer-session")
	if err != nil {
		t.Fatalf("creating authenticated session failed: %v", err)
	}
	session.Values["authenticated"] = "test"
	if err := session.Save(request, sessionRecorder); err != nil {
		t.Fatalf("saving authenticated session failed: %v", err)
	}
	request.AddCookie(sessionRecorder.Result().Cookies()[0])

	recorder := httptest.NewRecorder()
	conf.SettingsHandler(p).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("settings post returned status %d; want %d", recorder.Code, http.StatusSeeOther)
	}

	data, err := os.ReadFile(filepath.Join(configPath, "config.json"))
	if err != nil {
		t.Fatalf("settings post did not write the config file: %v", err)
	}
	var saved Config
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("saved config is not valid JSON: %v", err)
	}
	if saved.Login.Username != "alex" {
		t.Errorf("saved username is %q; want %q", saved.Login.Username, "alex")
	}
	if !checkHash("a new password", saved.Login.Password) {
		t.Error("the new password was not persisted to the config file")
	}
}

// TestSettingsHandlerConcurrentWithReaders changes settings while other
// handlers read them, which is what happens whenever the operator saves the
// settings page while the viewer is loading media. Without the mutex the
// media directory - a struct holding a *url.URL - can be read half-updated.
func TestSettingsHandlerConcurrentWithReaders(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	configdir.Refresh()
	t.Cleanup(configdir.Refresh)
	if err := os.MkdirAll(filepath.Join(configHome, "pi-player"), 0o755); err != nil {
		t.Fatalf("creating temp config dir failed: %v", err)
	}

	mediaDir := t.TempDir()
	conf := &Config{
		Location: "PiPlayer",
		Mount:    mount{URL: sURL{URL: &url.URL{Path: mediaDir}}, Dir: mediaDir},
	}
	p := &Player{
		api:         &APIHandler{},
		conf:        conf,
		playlist:    &Playlist{},
		ConnViewer:  NewConnWS(),
		ConnControl: NewConnWS(),
	}
	settings := conf.SettingsHandler(p)
	content := http.HandlerFunc(contentHandler(p))

	cookie := authenticatedCookie(t)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range 20 {
			form := url.Values{
				"location": {fmt.Sprintf("Room %d", i)},
				"mountURL": {filepath.Join(mediaDir, fmt.Sprintf("sub%d", i))},
				"debug":    {"on"},
			}.Encode()
			request := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.AddCookie(cookie)
			settings.ServeHTTP(httptest.NewRecorder(), request)
		}
	}()

	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 20 {
				content.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/content/a.txt", nil))
				_ = conf.LocationName()
				_ = conf.Credentials()
				_ = conf.DebugEnabled()
			}
		}()
	}
	wg.Wait()
}

// authenticatedCookie returns a cookie for a logged-in session.
func authenticatedCookie(t *testing.T) *http.Cookie {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "http://piplayer.local/", nil)
	session, err := store.Get(request, "piplayer-session")
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
