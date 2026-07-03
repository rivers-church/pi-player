package piplayer

import (
	"log"
	"net/http"
)

// NewServer returns a new http.Server for the piplayer interface.
func NewServer(p *Player, addr string) *http.Server {
	mux := setupRoutes(p)
	serv := http.Server{Addr: addr, Handler: mux}

	return &serv
}

// setupRoutes registers the routes for the server.
func setupRoutes(p *Player) *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(p.api.statAssets))))
	// mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("pkg/piplayer/assets"))))
	mux.HandleFunc("/content/", etagWrapper(p))
	mux.HandleFunc("/login", LoginHandler(p))
	mux.HandleFunc("/logout", LogoutHandler)
	mux.HandleFunc("/control", p.HandleControl)
	mux.HandleFunc("/settings", p.conf.SettingsHandler(p))
	mux.HandleFunc("/viewer", p.HandleViewer)
	mux.HandleFunc("/ws/viewer", p.ConnViewer.HandlerWebsocket(p))
	mux.HandleFunc("/ws/control", p.ConnControl.HandlerWebsocket(p))
	mux.HandleFunc("/api", p.api.Handle(p))
	mux.HandleFunc("/api/dircheck", p.HandleDirCheck)
	mux.HandleFunc("/", p.api.handlerHome)

	return mux
}

// etagWrapper serves files from the currently configured media directory.
// The directory is read from the config on each request so that a media
// directory change from the settings page takes effect immediately, without
// needing to re-register routes or restart the server.
func etagWrapper(p *Player) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		fs := http.StripPrefix("/content/", http.FileServer(http.Dir(p.conf.Mount.Dir)))

		// TODO: calculate and set an Etag header for the requested content.

		fs.ServeHTTP(w, r)
	}
}

// Start the http server. This blocks until the server stops; a failure to
// listen is fatal, so the process exits and lets the service supervisor
// (systemd Restart=on-failure) decide whether to bring it back up.
func Start(plr *Player) {
	log.Printf("Listening on port %s\n", plr.Server.Addr)
	if err := plr.Server.ListenAndServe(); err != nil {
		log.Fatalf("ListenAndServe: %v", err)
	}
}
