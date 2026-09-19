package piplayer

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
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
	templates     *template.Template
}

// NewAPIHandler creates a new APIHandler. An error here means the embedded
// assets or templates are missing, which every page would then fail on, so it
// is reported rather than logged and worked around.
func NewAPIHandler(debug bool, test *string, statAssets, statTemplates embed.FS) (APIHandler, error) {
	subAssets, err := fs.Sub(statAssets, "pkg/piplayer/assets")
	if err != nil {
		return APIHandler{}, fmt.Errorf("error loading embedded assets: %w", err)
	}
	subTemplates, err := fs.Sub(statTemplates, "pkg/piplayer/templates")
	if err != nil {
		return APIHandler{}, fmt.Errorf("error loading embedded templates: %w", err)
	}

	// Parse every page once at startup rather than on each request. The
	// templates are embedded, so they can't change while the player runs, and
	// a broken one is a build problem worth failing on immediately.
	templates, err := template.ParseFS(subTemplates, "*.html")
	if err != nil {
		return APIHandler{}, fmt.Errorf("error parsing templates: %w", err)
	}

	return APIHandler{
		debug:         debug,
		test:          *test,
		statAssets:    subAssets,
		statTemplates: subTemplates,
		templates:     templates,
	}, nil
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
