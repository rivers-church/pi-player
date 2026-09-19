package piplayer

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
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
	Mount    mount
	Debug    bool
	Login    Login
	Remote   remote
}

// MediaDir returns the directory the media files are read from.
func (conf *Config) MediaDir() string {
	conf.mu.RLock()
	defer conf.mu.RUnlock()
	return conf.Mount.Dir
}

// MountURL returns the configured media directory URL.
func (conf *Config) MountURL() sURL {
	conf.mu.RLock()
	defer conf.mu.RUnlock()
	return conf.Mount.URL
}

// SetMount points the player at a new media directory and returns the
// directory it was using before.
func (conf *Config) SetMount(m mount) string {
	conf.mu.Lock()
	defer conf.mu.Unlock()
	old := conf.Mount.Dir
	conf.Mount = m
	return old
}

// LocationName returns the name this player is known by.
func (conf *Config) LocationName() string {
	conf.mu.RLock()
	defer conf.mu.RUnlock()
	return conf.Location
}

// SetLocation renames the player.
func (conf *Config) SetLocation(location string) {
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

// Credentials returns the login details of the single user in the system.
func (conf *Config) Credentials() Login {
	conf.mu.RLock()
	defer conf.mu.RUnlock()
	return conf.Login
}

// SetCredentials replaces the login details.
func (conf *Config) SetCredentials(l Login) {
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
			log.Printf("could not read embedded logo file, skipping copy: %v", err)
		} else if err := os.WriteFile(filepath.Join(mediaDir, "PiPlayer Logo.png"), logoFile, 0644); err != nil {
			log.Printf("could not write logo file to media dir: %v", err)
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
		login, _ := newLogin()

		// Set some default values for config.
		conf = &Config{
			Location: "PiPlayer",
			Mount: mount{
				URL: sURL{URL: &url.URL{Path: mediaDir}},
				Dir: mediaDir,
			},

			Debug:  true,
			Login:  login,
			Remote: remote{Names: []string{"keyboard"}},
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

	conf.Mount.Dir = conf.Mount.URL.Path
	return conf, nil
}

// Save marshalls the config struct and writes it to the real config file.
func (conf *Config) Save() error {
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

// SettingsHandler handles requests to the settings page
func (conf *Config) SettingsHandler(p *Player) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, loggedIn, err := CheckLogin(w, r)
		if err != nil {
			log.Println("error trying to retrieve session on login page:", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !loggedIn {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		if r.Method == http.MethodGet {
			mountURL := conf.MountURL()
			mu, err := url.PathUnescape(mountURL.String())
			if err != nil {
				log.Printf("SettingsHandler: Error unescaping URL '%s'\n", mountURL)
				mu = mountURL.String()
			}
			tempControl := TemplateHandler{
				filename:  "settings.html",
				templates: p.api.templates,
				data: map[string]any{
					"location": conf.LocationName(),
					"debug":    conf.DebugEnabled(),
					"username": conf.Credentials().Username,
					"mountURL": mu,
				},
			}
			tempControl.ServeHTTP(w, r)
			return
		}

		// process POST request
		if err := r.ParseForm(); err != nil {
			log.Println("Error trying to parse form in settings page.\n", err)
			http.Error(w, "Could not read the submitted form.", http.StatusBadRequest)
			return
		}
		location := r.PostFormValue("location")
		mountURL := r.PostFormValue("mountURL")
		username := r.PostFormValue("username")
		password := r.PostFormValue("password")
		debug := r.PostFormValue("debug")

		conf.SetDebug(debug == "on")

		if conf.DebugEnabled() {
			log.Printf("Received settings post: location: %s\nmountURL: %s\n", location, mountURL)
		}

		if location != "" {
			conf.SetLocation(location)
		}

		if username != "" && password != "" {
			hashed, err := hash(password)
			if err != nil {
				log.Println("error trying to encrypt password for saving", err)
				http.Error(w, "Could not save the new password.", http.StatusInternalServerError)
				return
			}

			conf.SetCredentials(Login{Username: username, Password: hashed})
		}

		// Persist the settings that don't need the media directory to change.
		if err := conf.Save(); err != nil {
			log.Println("error trying to save config file:", err)
		}

		if mountURL != "" {
			u, err := url.Parse(mountURL)
			if err != nil {
				log.Printf("Error parsing URL (%s)\n%v\n", mountURL, err)
			} else if u.Scheme == "smb" {
				log.Printf("SMB mounting no longer supported")
			} else if u.Scheme == "" {
				// Dir comes from Path, not String(), so a directory with a
				// space in it doesn't get stored percent-escaped.
				newMount := mount{URL: sURL{URL: u}, Dir: u.Path}

				// Compare the paths: sURL wraps a *url.URL, so comparing the
				// structs compares pointers and is never equal.
				if newMount.Dir != conf.MediaDir() {
					oldDir := conf.SetMount(newMount)
					if err := conf.Save(); err != nil {
						log.Println("error trying to save config:", err)
					}

					// Point the directory watcher at the new media dir.
					if p.playlist != nil && p.playlist.watcher != nil {
						if oldDir != "" {
							p.playlist.watcher.Remove(oldDir)
						}
						if exists(newMount.Dir) {
							if err := p.playlist.watcher.Add(newMount.Dir); err != nil {
								log.Println("error watching new media dir:", err)
							}
						}
					}

					// Tell the viewer and control page to reload the playlist
					// from the new directory instead of restarting the server.
					// (The control page also reloads via the redirect below.)
					reload := wsMessage{
						Component: "playlist",
						Event:     "newItems",
						Message:   "media directory changed. Get new items.",
					}
					p.ConnViewer.trySend(reload)
					p.ConnControl.trySend(reload)
				}
			}
		}

		http.Redirect(w, r, "/control", http.StatusSeeOther)
	}
}
