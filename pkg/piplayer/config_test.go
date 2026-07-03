package piplayer

import (
	"embed"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"
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
