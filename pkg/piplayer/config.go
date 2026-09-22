package piplayer

import (
	"crypto/rand"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/17xande/configdir"
)

// Config holds the configuration of the pi-player.
//
// The settings page changes these fields while other handlers are reading
// them - /content/ reads the media directory on every request - so mu guards
// everything below it. The fields stay exported because the config is
// marshalled to config.json; use the accessors rather than touching them
// directly once the server is running.
type Config struct {
	mu       sync.RWMutex
	Location string
	// MediaDir is where the player reads its media from.
	MediaDir string
	Debug    bool
	// Login must stay exported: it is marshalled to config.json.
	Login  login
	Remote remote
	// SessionKey signs the session cookies. It is generated per device on
	// first run, so a cookie minted on one player is worthless on another.
	SessionKey []byte
}

// sessionKeyLength is the size of a generated session signing key.
const sessionKeyLength = 32

// sessionKey returns the key used to sign session cookies.
func (conf *Config) sessionKey() []byte {
	conf.mu.RLock()
	defer conf.mu.RUnlock()
	return conf.SessionKey
}

// newSessionKey returns a fresh random key for signing session cookies.
func newSessionKey() ([]byte, error) {
	key := make([]byte, sessionKeyLength)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("error generating a session key: %w", err)
	}
	return key, nil
}

// mediaDir returns the directory the media files are read from.
func (conf *Config) mediaDir() string {
	conf.mu.RLock()
	defer conf.mu.RUnlock()
	return conf.MediaDir
}

// setMediaDir points the player at a new media directory and returns the
// directory it was using before.
func (conf *Config) setMediaDir(dir string) string {
	conf.mu.Lock()
	defer conf.mu.Unlock()
	old := conf.MediaDir
	conf.MediaDir = dir
	return old
}

// locationName returns the name this player is known by.
func (conf *Config) locationName() string {
	conf.mu.RLock()
	defer conf.mu.RUnlock()
	return conf.Location
}

// setLocation renames the player.
func (conf *Config) setLocation(location string) {
	conf.mu.Lock()
	defer conf.mu.Unlock()
	conf.Location = location
}

// DebugEnabled reports whether extra logging is turned on.
func (conf *Config) DebugEnabled() bool {
	conf.mu.RLock()
	defer conf.mu.RUnlock()
	return conf.Debug
}

// SetDebug turns extra logging on or off.
func (conf *Config) SetDebug(debug bool) {
	conf.mu.Lock()
	defer conf.mu.Unlock()
	conf.Debug = debug
}

// credentials returns the login details of the single user in the system.
func (conf *Config) credentials() login {
	conf.mu.RLock()
	defer conf.mu.RUnlock()
	return conf.Login
}

// setCredentials replaces the login details.
func (conf *Config) setCredentials(l login) {
	conf.mu.Lock()
	defer conf.mu.Unlock()
	conf.Login = l
}

// ConfigLoad reads the config file and unmarshalls it to the config struct.
// It resolves the real config and media directories and delegates to
// configLoadFromPath, which holds the testable core logic.
func ConfigLoad(statsAssets embed.FS) (*Config, error) {
	configPath := configdir.LocalConfig("pi-player")
	userHome, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("error trying to get user dir: %w", err)
	}

	mediaDir := filepath.Join(userHome, "Documents", "pi-player")

	return configLoadFromPath(configPath, mediaDir, statsAssets)
}

// configLoadFromPath loads (or first-time creates) the config at configPath,
// creating mediaDir and seeding it with the logo from assets if it's missing.
// Paths are injected so this can be exercised against temp dirs in tests.
func configLoadFromPath(configPath, mediaDir string, assets fs.FS) (*Config, error) {
	if _, err := os.Stat(mediaDir); os.IsNotExist(err) {
		// Create the media directory if it doesn't exist.
		if err := os.MkdirAll(mediaDir, 0744); err != nil {
			return nil, fmt.Errorf("error trying to create default pi-player dir: %w", err)
		}

		// Copy the logo file to the media directory (best effort — a missing
		// embedded asset shouldn't prevent the player from starting).
		if logoFile, err := fs.ReadFile(assets, "pkg/piplayer/assets/img/PiPlayer Logo.png"); err != nil {
			logger.Warn("could not read the embedded logo, skipping the copy", "error", err)
		} else if err := os.WriteFile(filepath.Join(mediaDir, "PiPlayer Logo.png"), logoFile, 0644); err != nil {
			logger.Warn("could not write the logo to the media directory", "error", err)
		}
	}

	// Create the config directory if it doesn't exist.
	if err := configdir.MakePath(configPath); err != nil {
		return nil, fmt.Errorf("error creating config dir: %w", err)
	}

	conf := &Config{}

	configFile := filepath.Join(configPath, "config.json")
	// Does the file not exist?
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		login, err := newLogin()
		if err != nil {
			return nil, fmt.Errorf("error creating the default login: %w", err)
		}

		sessionKey, err := newSessionKey()
		if err != nil {
			return nil, err
		}

		// Set some default values for config.
		conf = &Config{
			Location:   "PiPlayer",
			MediaDir:   mediaDir,
			Debug:      true,
			Login:      login,
			Remote:     remote{Names: []string{"keyboard"}},
			SessionKey: sessionKey,
		}

		if err := conf.saveToPath(configPath); err != nil {
			return nil, fmt.Errorf("error saving new config file: %w", err)
		}

		return conf, nil
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, conf)
	if err != nil {
		return nil, err
	}

	// A config written before the media directory was a plain string has
	// nothing here. Fall back to the default location, which is where a
	// standard install keeps its media anyway; anything custom gets re-entered
	// on the settings page.
	if conf.MediaDir == "" {
		logger.Warn("no media directory in the config, falling back to the default", "dir", mediaDir)
		conf.MediaDir = mediaDir
	}

	// Configs written before session keys were stored won't have one; give
	// this device its own. Sessions signed with the old hardcoded key stop
	// being accepted, so everyone logs in once more.
	if len(conf.SessionKey) == 0 {
		if conf.SessionKey, err = newSessionKey(); err != nil {
			return nil, err
		}
		if err := conf.saveToPath(configPath); err != nil {
			return nil, fmt.Errorf("error saving the new session key: %w", err)
		}
	}

	return conf, nil
}

// save marshalls the config struct and writes it to the real config file.
func (conf *Config) save() error {
	configPath := configdir.LocalConfig("pi-player")
	return conf.saveToPath(configPath)
}

// saveToPath writes the marshalled config to config.json under configPath.
func (conf *Config) saveToPath(configPath string) error {
	conf.mu.RLock()
	defer conf.mu.RUnlock()

	configFile := filepath.Join(configPath, "config.json")
	jconf, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configFile, jconf, 0600)
}
