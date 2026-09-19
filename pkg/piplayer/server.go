package piplayer

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
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

// requireLogin refuses the request unless it carries a logged-in session.
// Browsers are sent to the login page; the API gets a 401 it can act on.
func requireLogin(p *Player, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, loggedIn, _ := p.CheckLogin(w, r); !loggedIn {
			if p.conf.DebugEnabled() {
				log.Printf("refusing %s %s from %s: not logged in\n", r.Method, r.URL.Path, r.RemoteAddr)
			}
			if strings.HasPrefix(r.URL.Path, "/api") {
				http.Error(w, `{"success":false,"message":"not logged in"}`, http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next(w, r)
	}
}

// requireLoginOrLocal allows a logged-in session or a request from this
// machine. The kiosk browser opens the viewer on localhost without ever
// logging in, but nothing on the network should be able to read the media
// directory or take the display's socket away from it.
func requireLoginOrLocal(p *Player, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if isLoopback(r.RemoteAddr) {
			next(w, r)
			return
		}
		requireLogin(p, next)(w, r)
	}
}

// isLoopback reports whether the request came from this machine.
func isLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// setupRoutes registers the routes for the server.
func setupRoutes(p *Player) *http.ServeMux {
	mux := http.NewServeMux()

	// Patterns carry their method, so the mux answers anything else with a 405
	// and the handlers below don't have to check r.Method themselves.
	// Static assets carry nothing worth protecting and the login page needs
	// its stylesheet before anyone can log in.
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(p.api.statAssets))))

	login := LoginHandler(p)
	mux.HandleFunc("GET /login", login)
	mux.HandleFunc("POST /login", login)
	mux.HandleFunc("POST /logout", p.LogoutHandler)
	mux.HandleFunc("GET /{$}", p.handlerHome)

	settings := p.conf.SettingsHandler(p)
	mux.HandleFunc("GET /settings", requireLogin(p, settings))
	mux.HandleFunc("POST /settings", requireLogin(p, settings))

	mux.HandleFunc("GET /control", requireLogin(p, p.HandleControl))
	mux.HandleFunc("GET /ws/control", requireLogin(p, p.ConnControl.HandlerWebsocket(p)))
	mux.HandleFunc("POST /api", requireLogin(p, p.api.Handle(p)))
	mux.HandleFunc("GET /api/dircheck", requireLogin(p, p.HandleDirCheck))

	// The kiosk browser reaches these from localhost without a session.
	mux.HandleFunc("GET /content/", requireLoginOrLocal(p, contentHandler(p)))
	mux.HandleFunc("GET /viewer", requireLoginOrLocal(p, p.HandleViewer))
	mux.HandleFunc("GET /ws/viewer", requireLoginOrLocal(p, p.ConnViewer.HandlerWebsocket(p)))

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
