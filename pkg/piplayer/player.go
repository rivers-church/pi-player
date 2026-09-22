package piplayer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/gorilla/sessions"
)

// Player is the object that renders images to the screen through chromium.
type Player struct {
	ConnViewer  ConnectionWS
	ConnControl ConnectionWS
	Server      *http.Server
	api         *APIHandler
	playlist    *Playlist
	conf        *Config
	store       *sessions.CookieStore
	browser     Browser
}

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
		logger.Error("no templates available to render the error page")
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}
	if tmplErr := p.api.templates.ExecuteTemplate(&page, "error.html", data); tmplErr != nil {
		logger.Error("rendering the error page failed", "error", tmplErr)
		http.Error(w, "Something went wrong.", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	if _, wErr := page.WriteTo(w); wErr != nil {
		logger.Error("writing the error page failed", "error", wErr)
	}
}

// NewPlayer creates a Player, starts its directory watcher and begins
// listening to the remote control.
// The context stops the remote control listener when the player shuts down.
func NewPlayer(ctx context.Context, api *APIHandler, conf *Config) *Player {
	p := Player{
		api:         api,
		conf:        conf,
		store:       newSessionStore(conf.sessionKey()),
		ConnViewer:  NewConnWS(),
		ConnControl: NewConnWS(),
	}

	var err error
	p.playlist, err = NewPlaylist(&p, conf.MediaDir())
	if err != nil {
		logger.Error("could not create the playlist", "error", err)
	}

	logger.Debug("initializing the remote control")
	go remoteRead(ctx, &p)

	return &p
}

// FirstRun starts the browser on a black screen and gets things going
func (p *Player) FirstRun() {
	if p.api.test == "web" {
		return
	}

	logger.Debug("starting the browser on first run")

	if err := p.startBrowser(); err != nil {
		logger.Error("could not start the browser", "error", err)
	}

	if len(p.playlist.snapshot().Items) == 0 {
		logger.Info("no items in the media directory")
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
	if logger.Enabled(context.Background(), slog.LevelDebug) {
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
			logger.Error("browser exited", "error", err)
			return
		}
		logger.Info("browser exited")
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
		logger.Error("could not ask the browser to quit", "error", err)
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
			logger.Error("could not close the directory watcher", "error", err)
		}
	}

	p.stopBrowser()
}

func handleAPIError(w http.ResponseWriter, status int, message string) {
	logger.Warn("api error", "message", message)
	writeAPIResponse(w, status, &resMessage{
		Success: false,
		Event:   "error",
		Message: message,
	})
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
		handleAPIError(w, http.StatusNotFound, "Method not supported: "+msg.Method)
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

	writeAPIResponse(w, http.StatusOK, &resMessage{
		Success: true,
		Event:   "StartRequestSent",
		Message: index,
	})
}

// HandleControl Scan the folder for new files every time the page reloads and display contents
func (p *Player) HandleControl(w http.ResponseWriter, r *http.Request) {
	if err := p.playlist.fromFolder(p.conf.MediaDir()); err != nil {
		logger.Error("could not read the media directory for the control page", "error", err)
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
		},
	}

	for _, item := range view.Items {
		logger.Debug("playlist item", "visual", item.Name(), "audio", item.Audio)
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
		logger.Error("could not read the session on the home page", "error", err)
		http.Error(w, "Could not read the session.", http.StatusInternalServerError)
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
			logger.Error("could not watch the media directory", "dir", dir, "error", err)
		}
	}
	if err := json.NewEncoder(w).Encode(map[string]bool{"ok": ok}); err != nil {
		logger.Error("writing the directory check response failed", "error", err)
	}
}

// HandleViewer handles requests to the image viewer page
// This handler has a dependency on Playlist.
func (p *Player) HandleViewer(w http.ResponseWriter, r *http.Request) {
	if err := p.playlist.fromFolder(p.conf.MediaDir()); err != nil {
		logger.Error("could not read the media directory for the viewer page", "error", err)
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
