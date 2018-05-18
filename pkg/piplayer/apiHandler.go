package piplayer

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

// APIHandler handles requests to the API
type APIHandler struct {
	debug   bool
	test    string
	message reqMessage
}

// NewAPIHandler creates a new APIHandler
func NewAPIHandler(debug bool, test *string) APIHandler {
	return APIHandler{debug: debug, test: *test}
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
		p.playlist.handleAPI(p.api, w, r)
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
