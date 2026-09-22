package piplayer

import (
	"net/http"

	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"
)

// login holds the credentials to the single user in the system.
type login struct {
	Username string
	Password string
}

// newSessionStore builds the cookie store that signs session cookies. The key
// comes from the config, where it is generated per device on first run: a key
// hardcoded here would be public in the repo, and anyone could then mint a
// cookie that says they're logged in.
func newSessionStore(key []byte) *sessions.CookieStore {
	store := sessions.NewCookieStore(key)
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 30,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		// PiPlayer is normally administered over its local HTTP server.
		Secure: false,
	}
	store.MaxAge(store.Options.MaxAge)
	return store
}

// newLogin creates the default login credentials if none are found
func newLogin() (login, error) {
	p, err := hash("admin")
	if err != nil {
		return login{}, err
	}
	return login{Username: "admin", Password: p}, nil
}

// productionHashCost is the bcrypt work factor the player runs with. It is
// deliberately slow: about a second per hash on a low-powered device.
const productionHashCost = 14

// hashCost is what hash() actually uses. Tests lower it - a suite that hashes
// a dozen passwords at the production cost spends all its time in bcrypt, and
// an order of magnitude more than that under the race detector.
var hashCost = productionHashCost

func hash(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), hashCost)
	return string(bytes), err
}

func checkHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// checkLogin checks if the user is logged in
func (p *Player) checkLogin(w http.ResponseWriter, r *http.Request) (*sessions.Session, bool, error) {
	session, err := p.store.Get(r, "piplayer-session")
	if err != nil {
		if session == nil {
			return nil, false, err
		}
		// A cookie that can't be decoded (signed with an older secret, tampered
		// with, or past the codec's MaxAge) isn't a server error: the store
		// still hands back a usable, empty session. Treat it as "not logged in"
		// so the user gets the login page instead of a 500 on every route,
		// which would lock them out until they cleared their cookies by hand.
		logger.Info("discarding a session cookie that could not be decoded", "error", err)
		return session, false, nil
	}

	_, authenticated := session.Values["authenticated"]

	return session, authenticated, nil
}

// loginHandler handles login requests. It accepts the config save operation
// separately so first-run login behaviour can be tested without writing to the
// user's real config directory.
func loginHandler(p *Player, saveConfig func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, loggedIn, err := p.checkLogin(w, r)
		if err != nil {
			logger.Error("could not read the session on the login page", "error", err)
			http.Error(w, "Could not read the session.", http.StatusInternalServerError)
			return
		}

		// if already logged in the user has already been redirected.
		// rest of logic can be ignored.
		if loggedIn {
			http.Redirect(w, r, "/control", http.StatusFound)
			return
		}

		if r.Method == http.MethodGet {
			tempControl := templateHandler{
				filename:  "login.html",
				templates: p.api.templates,
				data: map[string]interface{}{
					"location": p.conf.locationName(),
				},
			}
			tempControl.ServeHTTP(w, r)
			return
		}

		// process POST request
		xForward := r.Header.Get("x-forwarded-for")
		logger.Debug("login attempt", "remoteAddr", r.RemoteAddr, "xForwardedFor", xForward)
		if err := r.ParseForm(); err != nil {
			logger.Warn("could not parse the login form", "error", err)
		}
		username := r.PostFormValue("username")
		password := r.PostFormValue("password")

		// if there's no login entry in the config file, add the default login details
		creds := p.conf.credentials()
		if creds.Username == "" {
			logger.Info("no login details in the config file, creating the default ones")
			var err error
			if creds, err = newLogin(); err != nil {
				logger.Error("could not create the default login", "error", err)
				http.Error(w, "Could not create the default login.", http.StatusInternalServerError)
				return
			}
			p.conf.setCredentials(creds)
			if err := saveConfig(); err != nil {
				logger.Error("could not save the config file", "error", err)
			}
		}

		if username == creds.Username && checkHash(password, creds.Password) {
			// user successfully logged in
			logger.Info("login successful", "remoteAddr", r.RemoteAddr)

			// Start from a clean set of values rather than keeping whatever
			// the incoming cookie carried. The session lives entirely in the
			// signed cookie, so this is the whole of it.
			session.Values = map[any]any{"authenticated": r.RemoteAddr}
			if err := session.Save(r, w); err != nil {
				logger.Error("could not save the login session", "error", err)
				http.Error(w, "Could not start the session.", http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, "/control", http.StatusFound)
			return
		}

		tempControl := templateHandler{
			templates: p.api.templates,
			filename:  "login.html",
			data: map[string]interface{}{
				"location":     p.conf.locationName(),
				"flashMessage": "Incorrect username or password",
			},
		}
		tempControl.ServeHTTP(w, r)
	}
}

// handleLogout logs a user out and redirects them to the login page
func (p *Player) handleLogout(w http.ResponseWriter, r *http.Request) {
	// A cookie that can't be decoded still yields a usable session here, and
	// expiring it is exactly what logging out wants to do anyway.
	session, err := p.store.Get(r, "piplayer-session")
	if err != nil {
		logger.Warn("could not read the session on logout", "error", err)
	}
	if session == nil {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	// Empty the session before expiring it. The store signs whatever the
	// session holds into the replacement cookie, so leaving the values in
	// place hands the browser a cookie that still says "authenticated" and
	// relies on it to throw the cookie away.
	session.Values = map[any]any{}
	session.Options.MaxAge = -1
	if err := session.Save(r, w); err != nil {
		logger.Error("could not expire the session on logout", "error", err)
	}

	http.Redirect(w, r, "/login", http.StatusFound)
}
