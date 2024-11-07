package piplayer

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"

	"github.com/17xande/keylogger"
)

// Player2 represents the entire program. It's the shell that holds the
// components together. The other components are the Streamer, Playlist,
// and Remote
type Player2 interface {
	Start(i Item)
	Stop() error
	Next() error
	Previous() error
	Listen(s chan string)
}

// Player is the object that renders images to the screen through omxplayer or chromium
type Player struct {
	ConnViewer  ConnectionWS
	ConnControl ConnectionWS
	Server      *http.Server
	// serveMux    *http.ServeMux
	api *APIHandler
	// command     *exec.Cmd
	// pipeIn      io.WriteCloser
	playlist *Playlist
	conf     *Config
	// running     bool
	// quitting    bool
	// status      int
	// quit        chan error
	browser   Browser
	keylogger *keylogger.KeyLogger
	streamer  Streamer
}

const (
// statusMenu = 1
// statusLive = 0
)

// Browser represents the chromium process that is used to display web pages and still images to the screen
type Browser struct {
	command *exec.Cmd
	running bool
	ctxt    *context.Context
	cancel  *context.CancelFunc
}

// var commandList = map[string]string{
// 	"speedIncrease":   "1",
// 	"speedDecrease":   "2",
// 	"rewind":          "<",
// 	"fastForward":     ">",
// 	"chapterPrevious": "i",
// 	"chapterNext":     "o",
// 	"exit":            "q",
// 	"quit":            "q",
// 	"pauseResume":     "p",
// 	"volumeDecrease":  "-",
// 	"volumeIncrease":  "+",
// 	"seekBack30":      "\x1b[D",
// 	"seekForward30":   "\x1b[C",
// 	"seekBack600":     "\x1b[B",
// 	"seekForward600":  "\x1b[A",
// }

// NewPlayer creates a new Player server *http.Server, router *mux.Router
func NewPlayer(api *APIHandler, conf *Config, keylogger *keylogger.KeyLogger) *Player {
	p := Player{
		api:         api,
		conf:        conf,
		keylogger:   keylogger,
		ConnViewer:  NewConnWS(),
		ConnControl: NewConnWS(),
		// TODO: Make this a config setting.
		streamer: &Chrome{
			ConnViewer:  &connWS{},
			ConnControl: &connWS{},
		},
	}

	var err error
	p.playlist, err = NewPlaylist(&p, conf.Mount.Dir)
	if err != nil {
		log.Printf("error trying to create playlist. Bailing out:\n%v\n", err)
		return nil
	}

	if api.debug {
		log.Println("initializing remote")
	}
	// TODO: get context from caller?
	go remoteRead(context.Background(), &p)

	// Listen for websocket messages from the browser.
	// go p.HandleWebSocketMessage()

	return &p
}

// FirstRun starts the browser on a black screen and gets things going
func (p *Player) FirstRun() {
	if p.api.test == "web" {
		return
	}

	if p.api.debug {
		log.Println("Starting browser on first run...")
	}

	if err := p.startBrowser(); err != nil {
		log.Println("Error trying to start the browser:\n", err)
		p.browser.running = false
	}

	if len(p.playlist.Items) == 0 {
		log.Println("No items in current directory.")
		return
	}

}

// Start the file that will be played in the browser. Sends a message to the
// ConnViewer channel to be sent over the websocket.
func (p *Player) Start(w *http.ResponseWriter) {
	// fileName, ok := p.api.message.Arguments["path"]
	sIndex, ok := p.api.message.Arguments["index"]
	if !ok {
		handleAPIError(w, "No intem index or provided")
		return
	}

	res := wsMessage{
		Event:   "start",
		Message: sIndex,
		Success: true,
	}

	send := p.ConnViewer.getChanSend()
	send <- res

	m := &resMessage{Success: true, Event: "StartRequestSent", Message: p.playlist.Current.Name()}
	json.NewEncoder(*w).Encode(m)
}

// startBrowser starts Chromium browser, or Google Chrome with the relevant flags.
func (p *Player) startBrowser() error {
	if p.browser.running {
		return errors.New("error: Browser already running, cannot start another instance")
	}

	// https://peter.sh/experiments/chromium-command-line-switches/
	flags := []string{
		// "--no-user-gesture-required",
		"-kiosk",
		// "-private-window",
		"http://localhost:8080/viewer",
	}

	browser := "firefox"

	if p.api.test == "linux" {
		flags = []string{
			"--incognito",
			"http://localhost:8080/viewer",
		}

		browser = "google-chrome"
	} else if p.api.test == "mac" {
		flags = []string{
			"--incognito",
			"http://localhost:8080/viewer",
		}

		browser = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	}

	p.browser.command = exec.Command(browser, flags...)
	p.browser.command.Stdin = os.Stdin
	if p.api.debug {

		p.browser.command.Stdout = os.Stdout
	}
	p.browser.command.Stderr = os.Stderr
	if err := p.browser.command.Start(); err != nil {
		return err
	}
	p.browser.running = true

	ctxt, cancel := context.WithCancel(context.Background())
	p.browser.ctxt = &ctxt
	p.browser.cancel = &cancel
	// not sure if this is appropriate here. Not sure if context is
	// absolutely needed actually. I'm not gracefully terminating things
	// defer cancel()

	return nil
}

// Next goes to the next item in the playlist.
func (p *Player) Next() error {
	// TODO: everything
	return nil
}

// Previous goes to the previous item in the playlist.
func (p *Player) Previous() error {
	// TODO: everything
	return nil
}

func handleAPIError(w *http.ResponseWriter, message string) {
	m := &resMessage{
		Success: false,
		Message: message,
	}

	log.Println(m)
	json.NewEncoder(*w).Encode(m)
}

// Listen should listen to something, I forgot what
func (p *Player) Listen(s chan string) {
	// TODO: everything
}

// Handles requets to the player api
func (p *Player) ServeHTTP(w http.ResponseWriter, h *http.Request) {
	supportedAPIMethods := map[string]bool{
		"start":    true,
		"stop":     true,
		"play":     true,
		"pause":    true,
		"seek":     true,
		"next":     true,
		"previous": true,
	}

	if _, ok := supportedAPIMethods[p.api.message.Method]; !ok {
		handleAPIError(&w, "Method not supported: "+p.api.message.Method)
		return
	}

	index := p.api.message.Arguments["index"]

	res := wsMessage{
		Component: p.api.message.Component,
		Method:    p.api.message.Method,
		Arguments: p.api.message.Arguments,
		Event:     p.api.message.Method,
		Message:   index,
		Success:   true,
	}

	send := p.ConnViewer.getChanSend()
	send <- res

	m := &resMessage{Success: true, Event: "StartRequestSent", Message: index}
	json.NewEncoder(w).Encode(m)
}

// HandleControl Scan the folder for new files every time the page reloads and display contents
func (p *Player) HandleControl(w http.ResponseWriter, r *http.Request) {
	_, loggedIn, err := CheckLogin(w, r)
	if err != nil {
		log.Println("error trying to retrieve session on login page:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !loggedIn {
		if p.conf.Debug {
			log.Println("User not logged in. Redirecting to login page.")
		}
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	err = p.playlist.fromFolder(p.conf.Mount.Dir)

	if err != nil {
		log.Println("HandleControl: Error trying to read files from directory:\n", err)
		t := template.Must(template.ParseFiles("pkg/piplayer/templates/error.html"))
		err := t.Execute(w, err)
		if err != nil {
			log.Println("Error trying to render error page. #fail.", err)
		}
		return
	}

	tempControl := TemplateHandler{
		filename:      "control.html",
		statTemplates: p.api.statTemplates,
		data: map[string]interface{}{
			"location": p.conf.Location,
			"Mount":    p.conf.Mount.URL,
			"playlist": p.playlist,
			"error":    err,
		},
	}

	if p.api.debug {
		log.Println("files in playlist:")
		for _, item := range p.playlist.Items {
			log.Printf("visual: %s", item.Name())
			if item.Audio != nil {
				log.Printf("\taudio: %s", item.Audio.Name())
			}
		}
	}

	// On every control page reload, send a message to the viewer
	// to refresh the items playlist.
	msg := wsMessage{
		Component: "playlist",
		Event:     "newItems",
		Message:   "control page was refreshed. Get new items.",
	}

	send := p.ConnViewer.getChanSend()
	send <- msg
	tempControl.ServeHTTP(w, r)
}

// TODO: The Two handlers below only apply to the Chrome player, should they be moved
// the the chrome streamer file? surely not, because they belong to player right?

// HandleViewer handles requests to the image viewer page
// This handler has a dependency on Playlist.
func (p *Player) HandleViewer(w http.ResponseWriter, r *http.Request) {
	if err := p.playlist.fromFolder(p.conf.Mount.Dir); err != nil {
		log.Println("HandleViewer: Error trying to read files from directory:\n", err)
		t := template.Must(template.ParseFiles("pkg/piplayer/templates/error.html"))
		err := t.Execute(w, err)
		if err != nil {
			log.Println("Error trying to render error page. #fail.", err)
		}
		return
	}

	th := TemplateHandler{
		filename:      "viewer.html",
		statTemplates: p.api.statTemplates,
		data: map[string]interface{}{
			"playlist": p.playlist,
		},
	}

	th.ServeHTTP(w, r)
}
