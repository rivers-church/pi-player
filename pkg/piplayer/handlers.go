package piplayer

import (
	"net/http"
)

// handleSettings renders the settings page and applies what it submits.
func (p *Player) handleSettings() http.HandlerFunc {
	conf := p.conf

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			tempControl := templateHandler{
				filename:  "settings.html",
				templates: p.api.templates,
				data: map[string]any{
					"location": conf.locationName(),
					"debug":    conf.DebugEnabled(),
					"username": conf.credentials().Username,
					"mountURL": conf.mediaDir(),
				},
			}
			tempControl.ServeHTTP(w, r)
			return
		}

		// process POST request
		if err := r.ParseForm(); err != nil {
			logger.Warn("could not parse the settings form", "error", err)
			http.Error(w, "Could not read the submitted form.", http.StatusBadRequest)
			return
		}
		location := r.PostFormValue("location")
		mountURL := r.PostFormValue("mountURL")
		username := r.PostFormValue("username")
		password := r.PostFormValue("password")
		debug := r.PostFormValue("debug")

		conf.SetDebug(debug == "on")
		SetDebugLogging(conf.DebugEnabled())

		logger.Debug("settings submitted", "location", location, "mountURL", mountURL)

		if location != "" {
			conf.setLocation(location)
		}

		// The form only submits a password when one was typed, and leaving the
		// username alone is the common case - so take whichever of the two was
		// given and keep the other.
		if username != "" || password != "" {
			creds := conf.credentials()
			if username != "" {
				creds.Username = username
			}
			if password != "" {
				hashed, err := hash(password)
				if err != nil {
					logger.Error("could not hash the new password", "error", err)
					http.Error(w, "Could not save the new password.", http.StatusInternalServerError)
					return
				}
				creds.Password = hashed
			}

			conf.setCredentials(creds)
		}

		// Persist the settings that don't need the media directory to change.
		if err := conf.save(); err != nil {
			logger.Error("could not save the config file", "error", err)
		}

		if mountURL != "" && mountURL != conf.mediaDir() {
			oldDir := conf.setMediaDir(mountURL)
			if err := conf.save(); err != nil {
				logger.Error("could not save the config file", "error", err)
			}

			// Point the directory watcher at the new media dir.
			if p.playlist != nil && p.playlist.watcher != nil {
				if oldDir != "" {
					p.playlist.watcher.Remove(oldDir)
				}
				if exists(mountURL) {
					if err := p.playlist.watcher.Add(mountURL); err != nil {
						logger.Error("could not watch the new media directory", "dir", mountURL, "error", err)
					}
				}
			}

			// Tell the viewer and control page to reload the playlist from the
			// new directory instead of restarting the server. (The control page
			// also reloads via the redirect below.)
			reload := wsMessage{
				Component: "playlist",
				Event:     "newItems",
				Message:   "media directory changed. Get new items.",
			}
			p.ConnViewer.trySend(reload)
			p.ConnControl.trySend(reload)
		}

		http.Redirect(w, r, "/control", http.StatusSeeOther)
	}
}
