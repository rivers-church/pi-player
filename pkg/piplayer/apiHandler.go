package piplayer

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
)

// APIHandler handles requests to the API
type APIHandler struct {
	test          string
	statAssets    fs.FS
	statTemplates fs.FS
	templates     *template.Template
}

// NewAPIHandler creates a new APIHandler. An error here means the embedded
// assets or templates are missing, which every page would then fail on, so it
// is reported rather than logged and worked around.
func NewAPIHandler(test *string, statAssets, statTemplates embed.FS) (APIHandler, error) {
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
		test:          *test,
		statAssets:    subAssets,
		statTemplates: subTemplates,
		templates:     templates,
	}, nil
}

// writeAPIResponse sends an API response with a status code that matches what
// happened. The body shape stays the same either way, so the frontend's
// success checks keep working.
func writeAPIResponse(w http.ResponseWriter, status int, m *resMessage) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(m); err != nil {
		logger.Error("writing the API response failed", "error", err)
	}
}

// Handle handles all calls to the API
func (a *APIHandler) Handle(p *Player) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// ignore anything that's not a application/json request
		ct := r.Header.Get("Content-Type")
		if ct != "application/json" {
			logger.Warn("API request with an unsupported Content-Type", "contentType", ct)
			writeAPIResponse(w, http.StatusUnsupportedMediaType, &resMessage{
				Success: false,
				Event:   "invalidContentType",
				Message: "Invalid Content-Type: " + ct,
			})
			return
		}

		// Decode into a local: the APIHandler is shared by every request, so
		// keeping the message on it lets concurrent calls overwrite each other.
		var msg reqMessage
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			logger.Warn("could not decode the API request", "error", err)
			writeAPIResponse(w, http.StatusBadRequest, &resMessage{
				Success: false,
				Event:   "invalidJSON",
				Message: "Could not decode the JSON request.",
			})
			return
		}

		a.handleMessage(p, msg, w)
	}
}

func (a *APIHandler) handleMessage(p *Player, msg reqMessage, w http.ResponseWriter) {
	logger.Debug("API message received", "component", msg.Component, "method", msg.Method, "arguments", msg.Arguments)

	// dispatch execution based on which component was called
	switch msg.Component {
	case "player":
		p.handleAPI(msg, w)
		return
	case "playlist":
		p.playlist.handleAPI(msg, w, p.conf.mediaDir(), p.ConnControl)
		return
	}

	m := &resMessage{
		Success: false,
		Event:   "unsupportedComponent",
		Message: fmt.Sprintf("Unsupported component: %s (method: %s, arguments: %v)", msg.Component, msg.Method, msg.Arguments),
	}
	writeAPIResponse(w, http.StatusNotFound, m)
}
