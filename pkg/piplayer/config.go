package piplayer

import (
	"encoding/json"
	"fmt"
	"github.com/17xande/configdir"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

// Config holds the configuration of the pi-player
type Config struct {
	Location    string
	Mount       mount
	AudioOutput string
	Streamer    string
	Debug       bool
	Login       Login
	Remote      remote
}

// Load reads the config file and unmarshalls it to the config struct
func ConfigLoad(path string) (*Config, error) {
	configPath := configdir.LocalConfig("pi-player")
	userHome, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("error trying to get user dir: %w", err)
	}

	defaultDir := filepath.Join(userHome, "Documents", "pi-player")
	if err := os.MkdirAll(defaultDir, 0744); err != nil {
		return nil, fmt.Errorf("error trying to create default pi-player dir: %w", err)
	}

	// TODO: Put a pic with the Pi Player logo in the folder about so it displays on first load.

	// Create the directory if it doesn't exist.
	if err := configdir.MakePath(configPath); err != nil {
		return nil, fmt.Errorf("error creating config dir: %w", err)
	}

	conf := &Config{}

	configFile := filepath.Join(configPath, "config.json")
	// Does the file not exist?
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		// Create the file
		f, err := os.Create(configFile)
		defer f.Close()
		if err != nil {
			return nil, fmt.Errorf("error creating config file: %w", err)
		}

		login, _ := newLogin()

		// Set some default values for config.
		conf = &Config{
			Location: "PiPlayer",
			Mount: mount{
				URL: sURL{URL: &url.URL{Path: defaultDir}},
				Dir: defaultDir,
			},

			Debug:  true,
			Login:  login,
			Remote: remote{Names: []string{"keyboard"}},
		}

		conf.Save()

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
	conf.Mount.loadDir()

	return conf, nil
}

// Save reads the config struct, marshalls it and writes it to the config file
func (conf *Config) Save() error {
	configPath := configdir.LocalConfig("pi-player")
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
				data: map[string]interface{}{
					"location": conf.Location,
					// "directory":   conf.Directory,
					"audioOutput": conf.AudioOutput,
					"debug":       conf.Debug,
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
		mountDomain := r.PostFormValue("mountDomain")
		mountPassword := r.PostFormValue("mountPassword")
		audioOutput := r.PostFormValue("audioOutput")
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

		if audioOutput != "" {
			conf.AudioOutput = audioOutput
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

				if mountPassword != "" || mountUsername != "" {
					newMount.Username = mountUsername
					newMount.Domain = mountDomain
					newMount.Password = mountPassword
				}

				if newMount.URL != conf.Mount.URL ||
					newMount.Username != conf.Mount.Username ||
					newMount.Domain != conf.Mount.Domain ||
					newMount.Password != conf.Mount.Password {
					if su.Scheme == "smb" && !newMount.mounted() {
						if conf.Debug {
							log.Printf("Debug: Attempting to mount location: %s\n", newMount.Dir)
						}
						if err := newMount.mount(); err != nil {
							log.Printf("SettingsHandler: Error mounting new folder location:\n%s\n", err)
						} else {
							if conf.Debug {
								log.Printf("Debug: Mount for '%s' successful. Unmounting old '%s' mount.\n", newMount.Dir, conf.Mount.Dir)
							}
							// if the new folder was mounted successfully, unmount old folder.
							// if err := conf.Mount.unmount(); err != nil {
							// 	log.Printf("SettingsHandler: Error unmounting old directory:\n%s\n", err)
							// }
							conf.Mount = newMount
							if err := conf.Save(); err != nil {
								log.Println("error trying to save config:", err)
							}
							restart(p)
						}
					}

					if su.Scheme == "" {
						conf.Mount = newMount
						if err := conf.Save(); err != nil {
							log.Println("error trying to save config:", err)
						}
						restart(p)
					}
				}
			}
		}

		http.Redirect(w, r, "/control", http.StatusSeeOther)
	}
}
