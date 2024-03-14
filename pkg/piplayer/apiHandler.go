package piplayer

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
)

// APIHandler handles requests to the API
type APIHandler struct {
	debug         bool
	test          string
	message       reqMessage
	statAssets    fs.FS
	statTemplates fs.FS
}

// NewAPIHandler creates a new APIHandler
func NewAPIHandler(debug bool, test *string, statAssets, statTemplates embed.FS) APIHandler {
	subAssets, err := fs.Sub(statAssets, "pkg/piplayer/assets")
	if err != nil {
		if debug {
			log.Println("Error loading assets:", err)
		}
	}
	subTemplates, err := fs.Sub(statTemplates, "pkg/piplayer/templates")
	if err != nil {
		if debug {
			log.Println("Error loading templates:", err)
		}
	}
	return APIHandler{debug: debug, test: *test, statAssets: subAssets, statTemplates: subTemplates}
}

// Handles requests to the index page as well as any other requests
// that don't match any other paths
func (a *APIHandler) handlerHome(w http.ResponseWriter, r *http.Request) {

	if r.URL.Path != "/" {
		http.Error(w, "Not found", 404)
		log.Printf("Not found: %s", r.URL)
		return
	}
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		log.Printf("Method not allowed: %s", r.URL)
		return
	}
	_, loggedIn, err := CheckLogin(w, r)
	if err != nil {
		log.Println("error trying to retrieve session on login page:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if loggedIn {
		http.Redirect(w, r, "/control", http.StatusFound)
		return
	} else {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
}

// Handle handles all calls to the API
func (a *APIHandler) Handle(p *Player) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// ignore anything that's not a POST request
		if r.Method != "POST" {
			m := &resMessage{Success: false, Message: "Invalid request method: " + r.Method}
			log.Println(m.Message)
			json.NewEncoder(w).Encode(m)
			return
		}

		// ignore anything that's not a application/json request
		ct := r.Header.Get("Content-Type")
		if ct != "application/json" {
			m := &resMessage{Success: false, Message: "Invalid Content-Type: " + ct}
			log.Println(m.Message)
			json.NewEncoder(w).Encode(m)
			return
		}

		// decode message
		a.message = reqMessage{}
		decoder := json.NewDecoder(r.Body)
		err := decoder.Decode(&a.message)
		if err != nil {
			m := &resMessage{Success: false, Message: "Error decoding JSON request: " + err.Error()}
			log.Println(m.Message)
			json.NewEncoder(w).Encode(m)
			r.Body.Close()
			return
		}

		a.handleMessage(p, w, r)
	}
}

func (a *APIHandler) handleMessage(p *Player, w http.ResponseWriter, r *http.Request) {
	if a.debug {
		log.Printf("message received: %#v\n", a.message)
	}

	// displach execution based on which component was called
	// in this case, the Player component
	if a.message.Component == "player" {
		p.ServeHTTP(w, r)
		return
	}

	if a.message.Component == "playlist" {
		p.playlist.handleAPI(p, w, r)
		return
	}

	// return a generic success message for debugging
	m := &resMessage{
		Success: true,
		Message: fmt.Sprintf("Message Received:\ncomponent: %s\nmethod: %s\narguments: %v\n", a.message.Component, a.message.Method, a.message.Arguments),
	}
	json.NewEncoder(w).Encode(m)

	r.Body.Close()

	if p.api.debug {
		log.Println(m.Message)
	}
}
