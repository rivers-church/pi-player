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
	"path"
	"time"

	"github.com/17xande/keylogger"
	cdp "github.com/knq/chromedp"
)

// Player is the object that renders images to the screen through omxplayer or chromium-browser
type Player struct {
	Connection connectionWS
	api        *APIHandler
	command    *exec.Cmd
	pipeIn     io.WriteCloser
	playlist   playlist
	conf       *Config
	running    bool
	quitting   bool
	status     int
	quit       chan error
	browser    Browser
	keylogger  *keylogger.KeyLogger
}

const (
	statusMenu = 0
	statusLive = 1
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
	p := Player{api: api, conf: conf, keylogger: keylogger}
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

	err := p.playlist.fromFolder(p.conf.Directory)
	if err != nil {
		log.Println("Error trying to read files from directory.\n", err)
	}

	if len(p.playlist.Items) == 0 {
		log.Println("No items in current directory.")
		return
	}

	if p.api.debug {
		log.Println("trying to start first item in playlist:", p.playlist.Items[0].Name())
	}
	if err := p.Start(&p.playlist.Items[0], time.Duration(0)); err != nil {
		log.Println("Error trying to start first item in playlist.\n", err)
		return
	}
	p.playlist.Current = &p.playlist.Items[0]

	if p.api.debug {
		log.Println("first item should have started successfuly.")
	}

}

// Start the file that will be played to the screen, it decides which underlying program to use
// based on the type of file that will be opened.
func (p *Player) Start(item *Item, position time.Duration) error {
	var err error
	fileName := item.Name()

	if fileName == "" {
		return errors.New("empty fileName")
	}

	pos := fmt.Sprintf("%02d:%02d:%02d", int(position.Hours()), int(position.Minutes())%60, int(position.Seconds())%60)
	ext := path.Ext(fileName)

	// if omxplayer is already running, stop it
	if p.running {
		p.quitting = true
		p.quit = make(chan error)
		p.pipeIn.Write([]byte("q"))
		// block till omxplayer exits
		err := <-p.quit
		if err != nil && err.Error() != "exit status 3" {
			return err
		}
		close(p.quit)
	}

	if ext == ".mp4" {
		p.command = exec.Command("omxplayer", "-b", "-l", pos, "-o", p.conf.AudioOutput, path.Join(p.conf.Directory, fileName))
		// check if video must be looped
		if len(fileName) > 8 && fileName[len(fileName)-8:len(fileName)-4] == "LOOP" {
			p.command.Args = append(p.command.Args, "--loop")
		}
		p.pipeIn, err = p.command.StdinPipe()
		if err != nil {
			return err
		}

		if p.api.test != "web" {
			if err := p.setBrowserBG(""); err != nil {
				log.Println("Error trying to set browser background to black before player video:\n", err)
			}
		}

		if p.api.test == "mac" {
			p.command = exec.Command("qlmanage", "-p", path.Join(p.conf.Directory, fileName))
		} else if p.api.test == "linux" {
			p.command = exec.Command("xdg-open", path.Join(p.conf.Directory, fileName))
		} else if p.api.test == "web" {
			p.command = exec.Command("echo", path.Join(p.conf.Directory, fileName))
		}

		if p.api.debug {
			p.command.Stdout = os.Stdout
		}
		p.command.Stderr = os.Stderr

		err := p.command.Start()
		if err != nil {
			return err
		}
		p.running = true
		p.status = statusLive

		// wait for program to finish
		go func() {
			// Cmd.Wait() blocks till the process is finished
			err := p.command.Wait()
			p.running = false
			if p.quitting {
				if p.api.debug {
					log.Println("quitting the player")
				}
				p.quitting = false
				p.quit <- err
				return
			}
			if p.api.debug {
				log.Println("player wasn't quitting, go to next item")
			}
			// if the process was not quit midway, and ended naturally, go to the next item.
			if err := p.next(); err != nil {
				log.Printf("Error trying to go to next item after current item finished: %v\n", err)
			}

		}()
	} else if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".html" {
		if p.api.test == "web" {
			log.Println("ignore request to display image.")
			return nil
		}
		if !p.browser.running {
			log.Println("Error: Browser should be running by now. Trying to start it again")
			if err := p.startBrowser(); err != nil {
				return errors.New("Could not start browser again\n" + err.Error())
			}
		}

		err = p.setBrowserBG(fileName)
		if err != nil {
			return err
		}

		if item.Audio != nil {
			if res, err := p.startBrowserAudio(item.Audio.Name()); err != nil {
				log.Printf("Error trying to start audio:\n%s\nReturned Message:\n%v", err, res)
			}
		} else {
			if res, err := p.stopBrowserAudio(); err != nil {
				log.Printf("Error trying to start audio:\n%s\nReturned Message:\n%v", err, res)
			}
		}
	}

	return err
}

func (p *Player) setBrowserBG(url string) error {
	v := "background-image: url('/content/" + url + "')"
	return p.browser.cdp.Run(*p.browser.ctxt, cdp.SetAttributeValue("#container", "style", v, cdp.ByID))
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

	if p.api.test == "linux" {
		time.Sleep(15 * time.Second)
	} else {
		time.Sleep(7 * time.Second)
	}

	if p.api.debug {
		p.browser.cdp, err = cdp.New(ctxt, cdp.WithLog(log.Printf))
	} else {
		p.browser.cdp, err = cdp.New(ctxt)
	}

	if err != nil {
		err = errors.New("Error trying to start cdp:\n" + err.Error())
	}

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
		t := template.Must(template.ParseFiles("pkg/piPlayer/templates/error.html"))
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

	th := TemplateHandler{
		filename: "viewer.html",
		data: map[string]interface{}{
			"img": "/content/" + q.Get("img"),
		},
	}

	th.ServeHTTP(w, r)
}

// HandleMenu handles requests to the menu page
func (p *Player) HandleMenu(w http.ResponseWriter, r *http.Request) {
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
