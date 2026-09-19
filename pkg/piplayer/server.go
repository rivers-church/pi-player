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

	// Patterns carry their method, so the mux answers anything else with a 405
	// and the handlers below don't have to check r.Method themselves.
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(p.api.statAssets))))
	mux.HandleFunc("GET /content/", contentHandler(p))

	login := LoginHandler(p)
	mux.HandleFunc("GET /login", login)
	mux.HandleFunc("POST /login", login)
	mux.HandleFunc("POST /logout", LogoutHandler)

	settings := p.conf.SettingsHandler(p)
	mux.HandleFunc("GET /settings", settings)
	mux.HandleFunc("POST /settings", settings)

	mux.HandleFunc("GET /control", p.HandleControl)
	mux.HandleFunc("GET /viewer", p.HandleViewer)
	mux.HandleFunc("GET /ws/viewer", p.ConnViewer.HandlerWebsocket(p))
	mux.HandleFunc("GET /ws/control", p.ConnControl.HandlerWebsocket(p))
	mux.HandleFunc("POST /api", p.api.Handle(p))
	mux.HandleFunc("GET /api/dircheck", p.HandleDirCheck)
	mux.HandleFunc("GET /{$}", p.api.handlerHome)

	return mux
}

// contentHandler serves files from the currently configured media directory.
// The directory is read from the config on each request so that a media
// directory change from the settings page takes effect immediately, without
// needing to re-register routes or restart the server.
func contentHandler(p *Player) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		fs := http.StripPrefix("/content/", http.FileServer(http.Dir(p.conf.Mount.Dir)))

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
