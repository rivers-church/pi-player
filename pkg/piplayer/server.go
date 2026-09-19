package piplayer

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"
)

const (
	readHeaderTimeout = 10 * time.Second
	readTimeout       = 30 * time.Second
	idleTimeout       = 120 * time.Second
	shutdownTimeout   = 5 * time.Second
	maxHeaderBytes    = 1 << 20
)

// NewServer returns a new http.Server for the piplayer interface.
func NewServer(p *Player, addr string) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           setupRoutes(p),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		// WriteTimeout is deliberately left unset. Nothing here streams, and
		// logging in takes about a second of bcrypt on a slow device.
		// Websockets are unaffected either way: gorilla clears the deadlines
		// on the hijacked connection once the handshake is done.
	}
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
	mux.HandleFunc("POST /logout", p.LogoutHandler)

	settings := p.conf.SettingsHandler(p)
	mux.HandleFunc("GET /settings", settings)
	mux.HandleFunc("POST /settings", settings)

	mux.HandleFunc("GET /control", p.HandleControl)
	mux.HandleFunc("GET /viewer", p.HandleViewer)
	mux.HandleFunc("GET /ws/viewer", p.ConnViewer.HandlerWebsocket(p))
	mux.HandleFunc("GET /ws/control", p.ConnControl.HandlerWebsocket(p))
	mux.HandleFunc("POST /api", p.api.Handle(p))
	mux.HandleFunc("GET /api/dircheck", p.HandleDirCheck)
	mux.HandleFunc("GET /{$}", p.handlerHome)

	return mux
}

// contentHandler serves files from the currently configured media directory.
// The directory is read from the config on each request so that a media
// directory change from the settings page takes effect immediately, without
// needing to re-register routes or restart the server.
func contentHandler(p *Player) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		fs := http.StripPrefix("/content/", http.FileServer(http.Dir(p.conf.MediaDir())))

		fs.ServeHTTP(w, r)
	}
}

// Run serves until ctx is cancelled or the listener fails, then shuts the
// server down gracefully. It returns nil on a clean shutdown.
func Run(ctx context.Context, plr *Player) error {
	log.Printf("Listening on port %s\n", plr.Server.Addr)

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- plr.Server.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	log.Println("Shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Shutdown doesn't wait for hijacked connections, so an idle websocket
	// can't hold this up.
	if err := plr.Server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return nil
}
