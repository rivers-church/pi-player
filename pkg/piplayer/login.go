package piplayer

import (
	"log"
	"net/http"

	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"
)

// Login holds the credentials to the single user in the system.
type Login struct {
	Username string
	Password string
}

// TODO: use a random environment variable instead of hard coding the secret here.
var store = newSessionStore()

func newSessionStore() *sessions.CookieStore {
	store := sessions.NewCookieStore([]byte("ip-player-session-secret"))
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
func newLogin() (Login, error) {
	p, err := hash("admin")
	if err != nil {
		return Login{}, err
	}
	return Login{Username: "admin", Password: p}, nil
}

func hash(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func checkHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// CheckLogin checks if the user is logged in
func CheckLogin(w http.ResponseWriter, r *http.Request) (*sessions.Session, bool, error) {
	session, err := store.Get(r, "piplayer-session")
	if err != nil {
		if session == nil {
			return nil, false, err
		}
		// A cookie that can't be decoded (signed with an older secret, tampered
		// with, or past the codec's MaxAge) isn't a server error: the store
		// still hands back a usable, empty session. Treat it as "not logged in"
		// so the user gets the login page instead of a 500 on every route,
		// which would lock them out until they cleared their cookies by hand.
		log.Println("discarding session cookie that could not be decoded:", err)
		return session, false, nil
	}

	_, authenticated := session.Values["authenticated"]

	return session, authenticated, nil
}

// LoginHandler handles login requests
func LoginHandler(p *Player) http.HandlerFunc {
	return loginHandler(p, p.conf.Save)
}

// loginHandler accepts the config save operation separately so first-run login
// behavior can be tested without writing to the user's real config directory.
func loginHandler(p *Player, saveConfig func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, loggedIn, err := CheckLogin(w, r)
		if err != nil {
			log.Println("error trying to retrieve session on login page:", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// if already logged in the user has already been redirected.
		// rest of logic can be ignored.
		if loggedIn {
			http.Redirect(w, r, "/control", http.StatusFound)
			return
		}

		if r.Method == http.MethodGet {
			tempControl := TemplateHandler{
				filename:      "login.html",
				statTemplates: p.api.statTemplates,
				data: map[string]interface{}{
					"location": p.conf.LocationName(),
				},
			}
			tempControl.ServeHTTP(w, r)
			return
		}

		// process POST request
		xForward := r.Header.Get("x-forwarded-for")
		if p.conf.DebugEnabled() {
			log.Println("attempted login request from:", xForward, r.RemoteAddr)
		}
		if err := r.ParseForm(); err != nil {
			log.Println("Error trying to parse form in login page.\n", err)
		}
		username := r.PostFormValue("username")
		password := r.PostFormValue("password")

		// if there's no login entry in the config file, add the default login details
		creds := p.conf.Credentials()
		if creds.Username == "" {
			if p.conf.DebugEnabled() {
				log.Println("no login details found in config file, creating default login details now.")
			}
			var err error
			if creds, err = newLogin(); err != nil {
				log.Println("error trying to save default username and password")
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			p.conf.SetCredentials(creds)
			if err := saveConfig(); err != nil {
				log.Println("error trying to save config file:", err)
			}
		}

		if username == creds.Username && checkHash(password, creds.Password) {
			// user successfully logged in
			if p.conf.DebugEnabled() {
				log.Printf("login successful from %s\n", r.RemoteAddr)
			}

			session.Values["authenticated"] = r.RemoteAddr
			if err := session.Save(r, w); err != nil {
				log.Println("error trying to save login session:", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, "/control", http.StatusFound)
			return
		}

		tempControl := TemplateHandler{
			statTemplates: p.api.statTemplates,
			filename:      "login.html",
			data: map[string]interface{}{
				"location":     p.conf.LocationName(),
				"flashMessage": "Incorrect username or password",
			},
		}
		tempControl.ServeHTTP(w, r)
	}
}

// LogoutHandler logs a user out and redirects them to the login page
func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	session, err := store.Get(r, "piplayer-session")
	if err != nil {
		log.Println("error trying to get session in logout page")
	}

	session.Options.MaxAge = -1
	if err := session.Save(r, w); err != nil {
		log.Println("error trying to set MaxAge on session to logout")
	}

	http.Redirect(w, r, "/login", http.StatusFound)
}
