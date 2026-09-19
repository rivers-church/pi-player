package piplayer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/17xande/keylogger"
	"github.com/gorilla/sessions"
)

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
	store    *sessions.CookieStore
	// running     bool
	// quitting    bool
	// status      int
	// quit        chan error
	browser   Browser
	keylogger *keylogger.KeyLogger
}

const (
// statusMenu = 1
// statusLive = 0
)

// Browser represents the chromium process that is used to display web pages
// and still images to the screen.
type Browser struct {
	mu      sync.Mutex
	command *exec.Cmd
	running bool
}

// isRunning reports whether the browser process is still up.
func (b *Browser) isRunning() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.running
}

type errorPageData struct {
	Error    string
	Dir      string
	IPs      []string
	Port     string
	Redirect string
}

func getLocalIPs() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				ips = append(ips, ipnet.IP.String())
			}
		}
	}
	return ips
}

// renderErrorPage shows the operator what went wrong and how to reach the
// player, with a 500 so the failure isn't reported to the browser as success.
func (p *Player) renderErrorPage(w http.ResponseWriter, err error, redirect string) {
	port := ""
	if p.Server != nil {
		port = strings.TrimPrefix(p.Server.Addr, ":")
	}
	data := errorPageData{
		Error:    err.Error(),
		Dir:      p.conf.MediaDir(),
		IPs:      getLocalIPs(),
		Port:     port,
		Redirect: redirect,
	}

	var page bytes.Buffer
	if p.api.templates == nil {
		log.Println("no templates available to render the error page")
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	if tmplErr := p.api.templates.ExecuteTemplate(&page, "error.html", data); tmplErr != nil {
		log.Println("Error rendering error page:", tmplErr)
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	if _, wErr := page.WriteTo(w); wErr != nil {
		log.Println("Error writing error page:", wErr)
	}
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

// NewPlayer creates a Player, starts its directory watcher and begins
// listening to the remote control.
// The context stops the remote control listener when the player shuts down.
func NewPlayer(ctx context.Context, api *APIHandler, conf *Config, keylogger *keylogger.KeyLogger) *Player {
	p := Player{
		api:         api,
		conf:        conf,
		keylogger:   keylogger,
		store:       newSessionStore(conf.sessionKey()),
		ConnViewer:  NewConnWS(),
		ConnControl: NewConnWS(),
	}

	var err error
	p.playlist, err = NewPlaylist(&p, conf.MediaDir())
	if err != nil {
		log.Printf("error creating playlist: %v\n", err)
	}

	if api.debug {
		log.Println("initializing remote")
	}
	go remoteRead(ctx, &p)

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
	}

	if len(p.playlist.snapshot().Items) == 0 {
		log.Println("No items in current directory.")
	}
}

// startBrowser starts Chromium browser, or Google Chrome with the relevant flags.
func (p *Player) startBrowser() error {
	if p.browser.isRunning() {
		return errors.New("error: Browser already running, cannot start another instance")
	}

	viewerURL := p.viewerURL()

	// https://peter.sh/experiments/chromium-command-line-switches/
	flags := []string{
		"--kiosk",
		"--enable-features=UseOzonePlatform",
		"--ozone-platform=wayland",
		"--autoplay-policy=no-user-gesture-required",
		"--disk-cache-dir=/dev/null", //this sets the cache store location to null
		"--aggressive-cache-discard", //in theory this clears the chrome cache
		viewerURL,
	}

	browser := "chromium"

	switch p.api.test {
	case "linux":
		flags = []string{
			"--incognito",
			viewerURL,
		}

		browser = "google-chrome"
	case "mac":
		flags = []string{
			"--incognito",
			viewerURL,
		}

		browser = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	}

	command := exec.Command(browser, flags...)
	if p.api.debug {
		command.Stdout = os.Stdout
	}
	command.Stderr = os.Stderr
	if err := command.Start(); err != nil {
		return err
	}

	p.browser.mu.Lock()
	p.browser.command = command
	p.browser.running = true
	p.browser.mu.Unlock()

	// Reap the process when it exits, so it doesn't linger as a zombie and so
	// the player knows the display is gone and can start it again.
	go func() {
		err := command.Wait()

		p.browser.mu.Lock()
		if p.browser.command == command {
			p.browser.running = false
		}
		p.browser.mu.Unlock()

		if err != nil {
			log.Printf("browser exited: %v\n", err)
			return
		}
		log.Println("browser exited")
	}()

	return nil
}

// viewerURL is the address the kiosk browser opens. It follows the port the
// server was actually given rather than assuming the default.
func (p *Player) viewerURL() string {
	port := "8080"
	if p.Server != nil {
		if _, serverPort, err := net.SplitHostPort(p.Server.Addr); err == nil && serverPort != "" {
			port = serverPort
		}
	}
	return "http://localhost:" + port + "/viewer"
}

// stopBrowser asks the browser process to quit.
func (p *Player) stopBrowser() {
	p.browser.mu.Lock()
	command := p.browser.command
	running := p.browser.running
	p.browser.mu.Unlock()

	if !running || command == nil || command.Process == nil {
		return
	}
	if err := command.Process.Signal(os.Interrupt); err != nil {
		log.Printf("error asking the browser to quit: %v\n", err)
	}
}

// Close shuts down everything the player owns: the browser, the directory
// watcher and both websocket connections.
func (p *Player) Close() {
	farewell := wsMessage{
		Component: "connection",
		Event:     "disconnect",
		Success:   true,
		Message:   "The player is shutting down.",
	}
	p.ConnViewer.closeCurrent(farewell)
	p.ConnControl.closeCurrent(farewell)

	if p.playlist != nil && p.playlist.watcher != nil {
		if err := p.playlist.watcher.Close(); err != nil {
			log.Printf("error closing the directory watcher: %v\n", err)
		}
	}

	p.stopBrowser()
}

func handleAPIError(w http.ResponseWriter, message string) {
	m := &resMessage{
		Success: false,
		Message: message,
	}

	log.Println(m)
	json.NewEncoder(w).Encode(m)
}

// handleAPI handles requests to the player api
func (p *Player) handleAPI(msg reqMessage, w http.ResponseWriter) {
	supportedAPIMethods := map[string]bool{
		"start":    true,
		"stop":     true,
		"play":     true,
		"pause":    true,
		"seek":     true,
		"next":     true,
		"previous": true,
	}

	if _, ok := supportedAPIMethods[msg.Method]; !ok {
		handleAPIError(w, "Method not supported: "+msg.Method)
		return
	}

	index := msg.Arguments["index"]

	res := wsMessage{
		Component: msg.Component,
		Method:    msg.Method,
		Arguments: msg.Arguments,
		Event:     msg.Method,
		Message:   index,
		Success:   true,
	}

	p.ConnViewer.trySend(res)

	m := &resMessage{Success: true, Event: "StartRequestSent", Message: index}
	json.NewEncoder(w).Encode(m)
}

// HandleControl Scan the folder for new files every time the page reloads and display contents
func (p *Player) HandleControl(w http.ResponseWriter, r *http.Request) {
	_, loggedIn, err := p.CheckLogin(w, r)
	if err != nil {
		log.Println("error trying to retrieve session on login page:", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !loggedIn {
		if p.conf.DebugEnabled() {
			log.Println("User not logged in. Redirecting to login page.")
		}
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}

	err = p.playlist.fromFolder(p.conf.MediaDir())

	if err != nil {
		log.Println("HandleControl: Error trying to read files from directory:\n", err)
		p.renderErrorPage(w, err, "/control")
		return
	}

	view := p.playlist.snapshot()

	tempControl := TemplateHandler{
		filename:  "control.html",
		templates: p.api.templates,
		data: map[string]any{
			"location": p.conf.LocationName(),
			"Mount":    p.conf.MountURL(),
			"playlist": view,
			"error":    err,
		},
	}

	if p.api.debug {
		log.Println("files in playlist:")
		for _, item := range view.Items {
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

	p.ConnViewer.trySend(msg)

	tempControl.ServeHTTP(w, r)
}

// handlerHome sends the browser to the control page or the login page,
// depending on whether it is logged in.
func (p *Player) handlerHome(w http.ResponseWriter, r *http.Request) {
	_, loggedIn, err := p.CheckLogin(w, r)
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

// HandleDirCheck returns whether the configured media directory currently exists.
func (p *Player) HandleDirCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	dir := p.conf.MediaDir()
	ok := exists(dir)
	if ok && p.playlist.watcher != nil {
		if err := p.playlist.watcher.Add(dir); err != nil {
			log.Println("HandleDirCheck: error adding watcher:", err)
		}
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": ok})
}

// HandleViewer handles requests to the image viewer page
// This handler has a dependency on Playlist.
func (p *Player) HandleViewer(w http.ResponseWriter, r *http.Request) {
	if err := p.playlist.fromFolder(p.conf.MediaDir()); err != nil {
		log.Println("HandleViewer: Error trying to read files from directory:\n", err)
		p.renderErrorPage(w, err, "/viewer")
		return
	}

	th := TemplateHandler{
		filename:  "viewer.html",
		templates: p.api.templates,
		data: map[string]any{
			"playlist": p.playlist.snapshot(),
		},
	}

	th.ServeHTTP(w, r)
}
