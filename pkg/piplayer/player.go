package piplayer

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"io"
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
	serveMux    *http.ServeMux
	api         *APIHandler
	command     *exec.Cmd
	pipeIn      io.WriteCloser
	playlist    *Playlist
	conf        *Config
	running     bool
	quitting    bool
	status      int
	quit        chan error
	browser     Browser
	keylogger   *keylogger.KeyLogger
	streamer    Streamer
}

const (
	statusMenu = 1
	statusLive = 0
)

// Browser represents the chromium process that is used to display web pages and still images to the screen
type Browser struct {
	command *exec.Cmd
	running bool
	ctxt    *context.Context
	cancel  *context.CancelFunc
}

var commandList = map[string]string{
	"speedIncrease":   "1",
	"speedDecrease":   "2",
	"rewind":          "<",
	"fastForward":     ">",
	"chapterPrevious": "i",
	"chapterNext":     "o",
	"exit":            "q",
	"quit":            "q",
	"pauseResume":     "p",
	"volumeDecrease":  "-",
	"volumeIncrease":  "+",
	"seekBack30":      "\x1b[D",
	"seekForward30":   "\x1b[C",
	"seekBack600":     "\x1b[B",
	"seekForward600":  "\x1b[A",
}

// NewPlayer creates a new Player server *http.Server, router *mux.Router
func NewPlayer(api *APIHandler, conf *Config, keylogger *keylogger.KeyLogger) *Player {
	p := Player{
		api:         api,
		conf:        conf,
		keylogger:   keylogger,
		ConnViewer:  ConnectionWS{},
		ConnControl: ConnectionWS{},
		playlist:    &Playlist{Name: conf.Mount.URL.String()},
		// TODO: Make this a config setting.
		streamer: &Chrome{
			ConnViewer:  ConnectionWS{},
			ConnControl: ConnectionWS{},
		},
	}

	if api.debug {
		log.Println("initializing remote")
	}

	chans, err := keylogger.Read()
	if err != nil {
		log.Println("error trying to read the remote files: ", err)
	}

	for _, ie := range chans {
		go remoteRead(&p, ie)
	}

	// Listen for websocket messages from the browser.
	go p.HandleWebSocketMessage()

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

	p.ConnViewer.send <- res

	m := &resMessage{Success: true, Event: "StartRequestSent", Message: p.playlist.Current.Name()}
	json.NewEncoder(*w).Encode(m)
	return
}

// startBrowser starts Chromium browser, or Google Chrome with the relevant flags.
func (p *Player) startBrowser() error {
	if p.browser.running {
		return errors.New("Error: Browser already running, cannot start another instance")
	}

	flags := []string{
		"--window-size=1920,1080",
		"--window-position=0,0",
		"--kiosk",
		"--incognito",
		"--disable-infobars",
		"--noerrdialogs",
		"--no-first-run",
		"--enable-experimental-web-platform-features",
		"--javascript-harmony",
		"--autoplay-policy=no-user-gesture-required",
		"--remote-debugging-port=9222",

		// Experimental gpu enabling flags for higher video playback performance
		/*
			"--ignore-gpu-blacklist",
			"--enable-gpu-rasterization",
			"--enable-native-gpu-memory-buffers",
			"--enable-checker-imaging",
			"--disable-quic",
			"--enable-tcp-fast-open",
			"--disable-gpu-compositing",
			"--enable-fast-unload",
			"--enable-experimental-canvas-features",
			"--enable-scroll-prediction",
			"--enable-simple-cache-backend",
			"--answers-in-suggest",
			"--ppapi-flash-path=/usr/lib/chromium-browser/libpepflashplayer.so",
			"--ppapi-flash-args=enable_stagevideo_auto=0",
			"--ppapi-flash-version=",
			"--max-tiles-for-interest-area=512",
			"--num-raster-threads=4",
			"--default-tile-height=512",
		*/
		// End of experimental flags

		"http://localhost:8080/viewer",
	}

	browser := "chromium"

	if p.api.test == "linux" {
		flags = []string{
			"--incognito",
			"--remote-debugging-port=9222",
			"http://localhost:8080/viewer",
		}

		browser = "google-chrome"
	} else if p.api.test == "mac" {
		flags = []string{
			"--incognito",
			"--remote-debugging-port=9222",
			"http://localhost:8080/viewer",
		}

		browser = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	}

	p.browser.command = exec.Command(browser, flags...)
	if p.api.test == "" {
		p.browser.command.Env = []string{"DISPLAY=:0.0"}
	}

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
	return
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

	p.ConnViewer.send <- res

	m := &resMessage{Success: true, Event: "StartRequestSent", Message: index}
	json.NewEncoder(w).Encode(m)
	return
}

// HandleTest susan
func (p *Player) HandleTest(w http.ResponseWriter, r *http.Request) {
	err := p.playlist.fromFolder(p, p.conf.Mount.Dir)

	if err != nil {
		log.Println("HandleTest: Error trying to read files from directory:\n", err)
		t := template.Must(template.ParseFiles("pkg/piplayer/templates/error.html"))
		err := t.Execute(w, err)
		if err != nil {
			log.Println("Error trying to render error page. #fail.", err)
		}
		return
	}

	tempTest := TemplateHandler{
		filename: "test.html",
		data: map[string]interface{}{
			"location": p.conf.Location,
			"Mount":    p.conf.Mount.URL,
			"playlist": p.playlist,
			"error":    err,
		},
	}
	tempTest.ServeHTTP(w, r)
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
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	err = p.playlist.fromFolder(p, p.conf.Mount.Dir)

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
		filename: "control.html",
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

	p.ConnViewer.send <- msg
	tempControl.ServeHTTP(w, r)
}

// TODO: The Two handlers below only apply to the Chrome player, should they be moved
// the the chrome streamer file? surely not, because they belong to player right?

// HandleViewer handles requests to the image viewer page
// This handler has a dependency on Playlist.
func (p *Player) HandleViewer(w http.ResponseWriter, r *http.Request) {
	if err := p.playlist.fromFolder(p, p.conf.Mount.Dir); err != nil {
		log.Println("HandleViewer: Error trying to read files from directory:\n", err)
		t := template.Must(template.ParseFiles("pkg/piPlayer/templates/error.html"))
		err := t.Execute(w, err)
		if err != nil {
			log.Println("Error trying to render error page. #fail.", err)
		}
		return
	}

	th := TemplateHandler{
		filename: "viewer.html",
		data: map[string]interface{}{
			"playlist": p.playlist,
		},
	}

	th.ServeHTTP(w, r)
}

// HandleWebSocketMessage handles messages from ConnectionWS that come from the
// browser's websocket connection
// This handler has a dependency on the websockets usually attached to the player.
// the only streamer that will require this is the Chrome streamer.
// Wait, actually no, because the OMXStreamer needs to be used in tandem with the Chrome streamer...
// The VLC streamer shouldn't, so then should we have the option of multiple streamers???
func (p *Player) HandleWebSocketMessage() {
	if p.api.debug {
		log.Println("Listening to websocket messages from browser")
	}
	for {
		select {
		case msg, ok := <-p.ConnViewer.receive:
			if !ok {
				log.Println("Error receiving websocket message from viewer")
				return
			}

			if p.api.debug {
				log.Println("got a message from ConnectionWS", msg)
			}
		case msg, ok := <-p.ConnControl.receive:
			if !ok {
				log.Println("Error receiving websocket message from Control")
				return
			}

			if p.api.debug {
				log.Println("got a message from ConnectionWS", msg)
			}
		}
	}
}
