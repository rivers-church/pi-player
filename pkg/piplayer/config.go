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

	"github.com/17xande/configdir"
)

// Config holds the configuration of the pi-player
type Config struct {
	Location string
	Mount    mount
	Debug    bool
	Login       Login
	Remote      remote
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

		if r.Method == "GET" {
			mu, err := url.PathUnescape(conf.Mount.URL.String())
			if err != nil {
				log.Printf("SettingsHandler: Error unescaping URL '%s'\n", conf.Mount.URL)
			}
			tempControl := TemplateHandler{
				filename:      "settings.html",
				statTemplates: p.api.statTemplates,
				data: map[string]any{
					"location": conf.Location,
					"debug":    conf.Debug,
					"username":    conf.Login.Username,
					"mount":       conf.Mount,
					"mountURL":    mu,
				},
			}
			tempControl.ServeHTTP(w, r)
			return
		} else if r.Method != "POST" {
			log.Println("Unsuported request type for Settings page:", r.Method)
			return
		}

		// process POST request
		if err := r.ParseForm(); err != nil {
			log.Println("Error trying to parse form in settings page.\n", err)
		}
		location := r.PostFormValue("location")
		mountURL := r.PostFormValue("mountURL")
		mountUsername := r.PostFormValue("mountUsername")
		mountPassword := r.PostFormValue("mountPassword")
		username := r.PostFormValue("username")
		password := r.PostFormValue("password")
		debug := r.PostFormValue("debug")

		conf.Debug = debug == "on"

		if conf.Debug {
			log.Printf("Received settings post: location: %s\nmountURL: %s\n", location, mountURL)
		}

		if location != "" {
			conf.Location = location
		}

		if username != "" && password != "" {
			var err error
			if password, err = hash(password); err != nil {
				log.Println("error trying to encrypt password for saving", err)
			} else {
				conf.Login.Username = username
				conf.Login.Password = password
			}
		}

		if mountURL != "" || mountPassword != "" && mountUsername != "" {
			var su sURL
			u, err := url.Parse(mountURL)
			if err != nil {
				log.Printf("Error parsing URL (%s)\n%v\n", mountURL, err)
			} else {
				su.URL = u
				newMount := mount{
					URL: su,
					Dir: su.URL.String(),
				}

				if newMount.URL != conf.Mount.URL {
					if su.Scheme == "smb" {
						if conf.Debug {
							log.Printf("SMB mounting no longer supported")
						}
					}

					if su.Scheme == "" {
						oldDir := conf.Mount.Dir
						conf.Mount = newMount
						if err := conf.Save(); err != nil {
							log.Println("error trying to save config:", err)
						}

						// Point the directory watcher at the new media dir.
						if oldDir != "" && oldDir != conf.Mount.Dir {
							p.playlist.watcher.Remove(oldDir)
						}
						if exists(conf.Mount.Dir) {
							if err := p.playlist.watcher.Add(conf.Mount.Dir); err != nil {
								log.Println("error watching new media dir:", err)
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
						if p.ConnViewer.isActive() {
							p.ConnViewer.getChanSend() <- reload
						}
						if p.ConnControl.isActive() {
							p.ConnControl.getChanSend() <- reload
						}
					}
				}
			}
		}

		http.Redirect(w, r, "/control", http.StatusSeeOther)
	}
}
