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

		// ignore anything that's not a application/json request
		ct := r.Header.Get("Content-Type")
		if ct != "application/json" {
			m := &resMessage{Success: false, Message: "Invalid Content-Type: " + ct}
			log.Println(m.Message)
			json.NewEncoder(w).Encode(m)
			return
		}

		// Decode into a local: the APIHandler is shared by every request, so
		// keeping the message on it lets concurrent calls overwrite each other.
		var msg reqMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			m := &resMessage{Success: false, Message: "Error decoding JSON request: " + err.Error()}
			log.Println(m.Message)
			json.NewEncoder(w).Encode(m)
			return
		}

		a.handleMessage(p, msg, w)
	}
}

func (a *APIHandler) handleMessage(p *Player, msg reqMessage, w http.ResponseWriter) {
	if a.debug {
		log.Printf("message received: %#v\n", msg)
	}

	// dispatch execution based on which component was called
	switch msg.Component {
	case "player":
		p.handleAPI(msg, w)
		return
	case "playlist":
		p.playlist.handleAPI(p, msg, w)
		return
	}

	// return a generic success message for debugging
	m := &resMessage{
		Success: true,
		Message: fmt.Sprintf("Message Received:\ncomponent: %s\nmethod: %s\narguments: %v\n", msg.Component, msg.Method, msg.Arguments),
	}
	json.NewEncoder(w).Encode(m)

	if a.debug {
		log.Println(m.Message)
	}
}
