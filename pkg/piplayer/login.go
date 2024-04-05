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

var store = sessions.NewCookieStore([]byte("ip-player-session-secret"))

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
		return nil, false, err
	}

	return session, session.Values["x-forwarded-for"] != nil, nil
}

// LoginHandler handles login requests
func LoginHandler(p *Player) http.HandlerFunc {
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

		if r.Method == "GET" {

			tempControl := TemplateHandler{
				filename:      "login.html",
				statTemplates: p.api.statTemplates,
				data: map[string]interface{}{
					"location": p.conf.Location,
				},
			}
			tempControl.ServeHTTP(w, r)
			return
		} else if r.Method != "POST" {
			log.Println("Unsuported request type for Settings page:", r.Method)
			return
		}

		// process POST request
		xForward := r.Header.Get("x-forwarded-for")
		if p.conf.Debug {
			log.Println("attempted login request from:", xForward, r.RemoteAddr)
		}
		if err := r.ParseForm(); err != nil {
			log.Println("Error trying to parse form in login page.\n", err)
		}
		username := r.PostFormValue("username")
		password := r.PostFormValue("password")

		// if there's no login entry in the config file, add the default login details
		if p.conf.Login.Username == "" {
			if p.conf.Debug {
				log.Println("no login details found in config file, creating default login details now.")
			}
			var err error
			if p.conf.Login, err = newLogin(); err != nil {
				log.Println("error trying to save default username and password")
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := p.conf.Save(""); err != nil {
				log.Println("error trying to save config file:", err)
			}
		}

		if username == p.conf.Login.Username && checkHash(password, p.conf.Login.Password) {
			// user successfully logged in
			session.Values["x-forwarded-for"] = xForward
			session.Save(r, w)
			http.Redirect(w, r, "/control", http.StatusFound)
			return
		}

		tempControl := TemplateHandler{
			statTemplates: p.api.statTemplates,
			filename:      "login.html",
			data: map[string]interface{}{
				"location":     p.conf.Location,
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
