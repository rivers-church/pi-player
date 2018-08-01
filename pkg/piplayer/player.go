package piplayer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/17xande/keylogger"
	cdp "github.com/knq/chromedp"
)

// Player is the object that renders images to the screen through omxplayer or chromium-browser
type Player struct {
	ConnViewer  ConnectionWS
	ConnControl ConnectionWS
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
}

const (
	statusMenu = 1
	statusLive = 0
)

// Browser represents the chromium-browser process that is used to display web pages and still images to the screen
type Browser struct {
	command *exec.Cmd
	running bool
	cdp     *cdp.CDP
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

// NewPlayer creates a new Player
func NewPlayer(api *APIHandler, conf *Config, keylogger *keylogger.KeyLogger) *Player {
	p := Player{
		api:         api,
		conf:        conf,
		keylogger:   keylogger,
		ConnViewer:  ConnectionWS{},
		ConnControl: ConnectionWS{},
		playlist:    &Playlist{Name: conf.Directory},
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

// CleanUp closes other components properly
func (p *Player) CleanUp() {
	// TODO: properly implement this cleanup function
	c := *p.browser.cancel
	c()
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

	// if p.api.debug {
	// 	log.Println("trying to start first item in playlist:", p.playlist.Items[0].Name())
	// }
	//
	// The first item will start from the browser now, so no need to call this code.
	// if err := p.Start(&p.playlist.Items[0], time.Duration(0)); err != nil {
	// 	log.Println("Error trying to start first item in playlist.\n", err)
	// 	return
	// }
	// p.playlist.Current = &p.playlist.Items[0]

	// if p.api.debug {
	// 	log.Println("first item should have started successfuly.")
	// }

}

// stop quits omxplayer if it's running.
func (p *Player) stop() error {
	if p.running {
		p.quitting = true
		p.quit = make(chan error)
		defer close(p.quit)
		p.pipeIn.Write([]byte("q"))
		// block till omxplayer exits
		err := <-p.quit
		if err != nil && err.Error() != "exit status 3" {
			return err
		}
	}
	return nil
}

// Start the file that will be played to the screen, it decides which underlying program to use
// based on the type of file that will be opened.
func (p *Player) Start(item *Item, position time.Duration) error {
	fileName := item.Name()

	if fileName == "" {
		return errors.New("empty fileName")
	}

	i := p.playlist.getIndex(fileName)

	res := resMessage{
		Event:   "start",
		Message: strconv.Itoa(i),
		Success: true,
	}

	p.ConnViewer.send <- res

	return nil
}

func (p *Player) setBrowserBG(url string) error {
	v := "background-image: url('/content/" + url + "')"
	return p.browser.cdp.Run(*p.browser.ctxt, cdp.SetAttributeValue("#container", "style", v, cdp.ByID))
}

func (p *Player) setBrowserLocation(url string) error {
	return p.browser.cdp.Run(*p.browser.ctxt, cdp.Navigate(url))
}

func (p *Player) startBrowserAudio(url string) (res interface{}, err error) {
	tasks := cdp.Tasks{
		cdp.SetAttributeValue("#audMusic", "src", "/content/"+url, cdp.ByID),
		// TODO: see if we can use something better than an interface for the response
		cdp.Evaluate(`audMusic.play();`, &res),
	}
	err = p.browser.cdp.Run(*p.browser.ctxt, tasks)
	return
}

func (p *Player) stopBrowserAudio() (res interface{}, err error) {
	tasks := cdp.Tasks{
		cdp.Evaluate(`audMusic.pause();`, &res),
	}
	// the response never returns properly here for some reason.
	err = p.browser.cdp.Run(*p.browser.ctxt, tasks)
	return
}

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
		"--remote-debugging-port=9222",
		"http://localhost:8080/viewer",
	}

	browser := "chromium-browser"

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
	var err error

	// start the Chrome Debugging Protocol thang
	// We have to wait for chrome to start running first,
	// so we the whole program has to take a little nap.
	// TODO: figure out a better way to do this. Perhaps put the initial
	// start of the first item in it's own goroutine that waits
	// till chrome starts outputting stuff?
	if p.api.debug {
		log.Println("taking a little nap to allow chrome to start nicely")
	}

	// if p.api.test == "linux" {
	// 	time.Sleep(15 * time.Second)
	// } else {
	// 	time.Sleep(10 * time.Second)
	// }

	// Not using Chrome Debug Protocol anymore.

	// time.Sleep(15 * time.Second)

	// // Start Chrome Debugging Protocol client
	// if p.api.debug {
	// 	p.browser.cdp, err = cdp.New(ctxt, cdp.WithLog(log.Printf))
	// } else {
	// 	p.browser.cdp, err = cdp.New(ctxt)
	// }

	// if err != nil {
	// 	err = errors.New("Error trying to start cdp:\n" + err.Error())
	// }

	return err
}

// SendCommand sends a command to the omxplayer process
func (p *Player) SendCommand(command string) error {
	cmd, ok := commandList[command]
	if !ok {
		return errors.New("Command not found: " + command)
	}

	var err error
	if p.api.debug {
		fmt.Println("cmd:", cmd)
	}
	if p.api.test == "mac" {
		return nil
	}

	// if the player isn't running ignore the comand, unless it is a play-pause
	// then start the current item again.
	if !p.running {
		if command == "pauseResume" {
			return p.Start(p.playlist.Current, 0)
		}
		return nil
	}

	b := []byte(cmd)
	_, err = p.pipeIn.Write(b)

	if err != nil {
		err = fmt.Errorf("sendCommand: %v", err)
	}

	return err
}

// Handles requets to the player api
func (p *Player) ServeHTTP(w http.ResponseWriter, h *http.Request) {
	if p.api.message.Method == "start" {
		fileName, ok := p.api.message.Arguments["path"]
		if !ok {
			m := &resMessage{Success: false, Message: "No movie name provided."}
			log.Println(m.Message)
			json.NewEncoder(w).Encode(m)
			return
		}

		var position = time.Duration(0)
		if pos, ok := p.api.message.Arguments["position"]; ok {
			p, err := time.ParseDuration(pos)
			if err != nil {
				m := &resMessage{Success: false, Message: "Error converting video position " + err.Error()}
				log.Println(m.Message)
				json.NewEncoder(w).Encode(m)
				return
			}

			position = p
		}

		i := p.playlist.getIndex(fileName)
		if i == -1 {
			m := &resMessage{Success: false, Message: "Trying to play a video that's not in the playlist: " + fileName}
			log.Println(m.Message)
			json.NewEncoder(w).Encode(m)
			return
		}

		err := p.Start(&p.playlist.Items[i], position)
		if err != nil {
			m := &resMessage{Success: false, Message: "Error trying to start video: " + err.Error()}
			log.Println(m.Message)
			json.NewEncoder(w).Encode(m)
			return
		}

		p.playlist.Current = &p.playlist.Items[i]

		m := &resMessage{Success: true, Event: "videoStarted", Message: p.playlist.Current.Name()}
		json.NewEncoder(w).Encode(m)
		return
	}

	if p.api.message.Method == "next" {
		err := p.next()
		if err != nil {
			m := &resMessage{Success: false, Message: "Error going to next video: " + err.Error()}
			log.Println(m.Message)
			json.NewEncoder(w).Encode(m)
			return
		}

		m := &resMessage{
			Success: true,
			Event:   "videoStarted",
			Message: p.playlist.Current.Name(),
		}
		json.NewEncoder(w).Encode(m)
		return
	}

	if p.api.message.Method == "previous" {
		err := p.previous()
		if err != nil {
			m := &resMessage{Success: false, Message: "Error going to previous video: " + err.Error()}
			log.Println(m.Message)
			json.NewEncoder(w).Encode(m)
			return
		}

		m := &resMessage{
			Success: true,
			Event:   "videoStarted",
			Message: p.playlist.Current.Name(),
		}
		json.NewEncoder(w).Encode(m)
		return
	}

	if p.api.message.Method == "sendCommand" {
		cmd, ok := p.api.message.Arguments["command"]
		if !ok {
			m := &resMessage{Success: false, Message: "No command sent."}
			log.Println(m.Message)
			json.NewEncoder(w).Encode(m)
			return
		}

		if cmd == "quit" {
			p.quitting = true
			p.quit = make(chan error)
		}

		err := p.SendCommand(cmd)
		if err != nil {
			m := &resMessage{Success: false, Message: "Error trying to execute command: " + err.Error()}
			log.Println(m.Message)
			json.NewEncoder(w).Encode(m)
			return
		}

		if cmd == "quit" {
			err := <-p.quit
			if err != nil {
				log.Println("error tring to stop video from web interface")
			}
			close(p.quit)
		}

		m := &resMessage{Success: true, Message: "Command sent and executed"}
		json.NewEncoder(w).Encode(m)
		return
	}

	m := &resMessage{Success: false, Message: "Method not supported"}
	log.Println(m.Message)
	json.NewEncoder(w).Encode(m)
	return
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

	err = p.playlist.fromFolder(p.conf.Directory)

	if err != nil {
		log.Println("Error tring to read files from directory: ", err)
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
			"location":  p.conf.Location,
			"directory": p.conf.Directory,
			"playlist":  p.playlist,
			"error":     err,
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
	tempControl.ServeHTTP(w, r)
}

// HandleViewer handles requests to the image viewer page
func (p *Player) HandleViewer(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	img := q.Get("img")

	// If no source image is supplied, load the first item in the playlist
	// Don't think this will work properly with videos yet.
	// Should probably handle this in the browser.
	// if img == "" {
	// 	if err := p.playlist.fromFolder(p.conf.Directory); err != nil {
	// 		log.Println("Can't read files from directory\n", err)
	// 	} else {
	// 		img = p.playlist.Items[0].Name()
	// 	}
	// }

	if err := p.playlist.fromFolder(p.conf.Directory); err != nil {
		log.Println("Error tring to read files from directory: ", err)
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
			"img":      "/content/" + img,
		},
	}

	th.ServeHTTP(w, r)
}

// HandleMenu handles requests to the menu page
func (p *Player) HandleMenu(w http.ResponseWriter, r *http.Request) {
	if err := p.playlist.fromFolder(p.conf.Directory); err != nil {
		log.Println("Error tring to read files from directory: ", err)
		t := template.Must(template.ParseFiles("pkg/piPlayer/templates/error.html"))
		err := t.Execute(w, err)
		if err != nil {
			log.Println("Error trying to render error page. #fail.", err)
		}
		return
	}

	th := TemplateHandler{
		filename: "menu.html",
		data: map[string]interface{}{
			"playlist": p.playlist,
			"img":      "/content/" + p.playlist.Items[0].Name(),
		},
	}

	p.status = statusMenu
	th.ServeHTTP(w, r)
}

// next starts the next item in the playlist
func (p *Player) next() error {
	n, err := p.playlist.getNext()
	if err != nil {
		return errors.New("can't go to next item:\n" + err.Error())
	}

	if p.api.debug {
		log.Println("going to next item: ", n.Name())
	}

	err = p.Start(n, time.Duration(0))

	if err == nil {
		p.playlist.Current = n
	}

	return err
}

func (p *Player) previous() error {
	n, err := p.playlist.getPrevious()
	if err != nil {
		return errors.New("can't go to previous item:\n" + err.Error())
	}

	if p.api.debug {
		log.Println("going to previous item: ", n.Name())
	}

	err = p.Start(n, time.Duration(0))

	if err == nil {
		p.playlist.Current = n
	}

	return err
}

func (p *Player) home() error {
	err := p.stop()
	if err != nil {
		return err
	}

	return p.setBrowserLocation("http://localhost:8080/menu")
}

// HandleWebSocketMessage handles messages from ConnectionWS that come from the
// browser's websocket connection
func (p *Player) HandleWebSocketMessage() {
	if p.api.debug {
		log.Println("Listening to websocket messages from browser")
	}
	for {
		select {
		case msg, ok := <-p.ConnViewer.receive:
			if !ok {
				log.Println("error receiving websocket message from viewer")
				return
			}

			if p.api.debug {
				log.Println("got a message from ConnectionWS", msg)
			}
		case msg, ok := <-p.ConnControl.receive:
			if !ok {
				log.Println("error receiving websocket message from Control")
				return
			}

			if p.api.debug {
				log.Println("got a message from ConnectionWS", msg)
			}
		}
	}
}
