package piplayer

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
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
func (conf *Config) Load(path string) error {
	if path == "" {
		path = "config.json"
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &conf)
}

// Save reads the config struct, marshalls it and writes it to the config file
func (conf *Config) Save(path string) error {
	if path == "" {
		path = "config.json"
	}

	jconf, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(path, jconf, 0600)
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

			tempControl := TemplateHandler{
				filename: "settings.html",
				data: map[string]interface{}{
					"location": conf.Location,
					// "directory":   conf.Directory,
					"audioOutput": conf.AudioOutput,
					"debug":       conf.Debug,
					"username":    conf.Login.Username,
					"mount":       conf.Mount,
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

		if conf.Debug {
			log.Printf("Received settings post: location: %s\n", location)
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

		if mountURL != "" && mountURL != conf.Mount.URL.String() {
			u, err := url.Parse(mountURL)
			var su sURL
			if err != nil {
				log.Printf("Error parsing URL (%s)\n%v\n", mountURL, err)
			} else {
				su.URL = u
				newMount := mount{
					URL:      su,
					Username: mountUsername,
					Domain:   mountDomain,
					Password: mountPassword,
				}

				if err := newMount.mount(); err != nil {
					log.Println("SettingsHandler: Error mounting new folder location:\n", err)
				} else {
					conf.Mount = newMount
				}
			}
			restart(p)
		}

		conf.Debug = debug == "on"

		if err := conf.Save(""); err != nil {
			log.Println("error trying to save config:", err)
		}

		http.Redirect(w, r, "/control", http.StatusSeeOther)
	}
}
