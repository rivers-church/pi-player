package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"

	"github.com/gorilla/websocket"
	"github.com/gvalkov/golang-evdev"
	cdp "github.com/knq/chromedp"
)

// Player is the object that renders images to the screen through omxplayer or chromium-browser
type Player struct {
	api      *apiHandler
	command  *exec.Cmd
	pipeIn   io.WriteCloser
	playlist playlist
	conf     config
	control  controller
	running  bool
	quitting bool
	quit     chan error
	browser  Browser
	remotes  []*evdev.InputDevice
}

// Browser represents the chromium-browser process that is used to display web pages and still images to the screen
type Browser struct {
	command *exec.Cmd
	running bool
}

// controller has the websocket connection to the control page
type controller struct {
	socket *websocket.Conn
	send   chan resMessage
}

const (
	socketBufferSize  = 1024
	messageBufferSize = 256
)

var upgrader = &websocket.Upgrader{ReadBufferSize: socketBufferSize, WriteBufferSize: socketBufferSize}

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

// Start the file that will be played to the screen, it decides which underlying program to use
// based on the type of file that will be opened.
func (p *Player) Start(fileName string, position time.Duration) error {
	var err error

	if fileName == "" {
		return errors.New("empty fileName")
	}

	pos := fmt.Sprintf("%02d:%02d:%02d", int(position.Hours()), int(position.Minutes())%60, int(position.Seconds())%60)
	ext := path.Ext(fileName)

	if p.api.test == "mac" {
		if p.running {
			p.command.Process.Kill()
			p.running = false
		}
		log.Println("running quick look with file...")
		p.command = exec.Command("qlmanage", "-p", path.Join(p.conf.Directory, fileName))
		p.command.Start()
		p.running = true

		return err
	} else if p.api.test == "linux" {
		if p.running {
			p.command.Process.Kill()
			p.running = false
		}
		log.Println("running the video on default linux player")
		p.command = exec.Command("xdg-open", path.Join(p.conf.Directory, fileName))
		p.command.Start()
		p.running = true

		return err
	}

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

		p.playlist.Current = nil
	}

	if ext == ".mp4" {
		p.command = exec.Command("omxplayer", "-b", "-l", pos, path.Join(p.conf.Directory, fileName))
		// check if video must be looped
		if len(fileName) > 8 && fileName[len(fileName)-8:len(fileName)-4] == "LOOP" {
			p.command.Args = append(p.command.Args, "--loop")
		}
		p.pipeIn, err = p.command.StdinPipe()
		if err != nil {
			return err
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

		// wait for program to finish
		go func() {
			// Cmd.Wait() blocks till the process is finished
			err := p.command.Wait()
			p.running = false
			if p.quitting {
				p.quit <- err
				close(p.quit)
				p.quitting = false
			} else { // if the process was not quit midway, and ended naturally, go to the next item.
				err := p.next()
				if err != nil {
					log.Printf("Error trying to go to next item after current item finished: %v\n", err)
				}
			}
		}()

	} else if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".html" {
		if !p.browser.running {
			f := []string{
				"--window-size=1920,1080",
				"--window-position=0,0",
				"--kiosk",
				"--incognito",
				"--disable-infobars",
				"--noerrdialogs",
				"--no-first-run",
				"--remote-debugging-port=9222",
				"http://localhost:8080/viewer?img=" + url.QueryEscape(fileName),
			}

			browser := "chromium-browser"

			p.browser.command = exec.Command(browser, f...)
			p.browser.command.Env = []string{"DISPLAY=:0.0"}

			p.browser.command.Stdin = os.Stdin
			if p.api.debug {
				p.browser.command.Stdout = os.Stdout
			}
			p.browser.command.Stderr = os.Stderr
			err = p.browser.command.Start()
			p.browser.running = true

		} else {
			ctxt, cancel := context.WithCancel(context.Background())
			defer cancel()

			var c *cdp.CDP

			if p.api.debug {
				c, err = cdp.New(ctxt, cdp.WithLog(log.Printf))
			} else {
				c, err = cdp.New(ctxt)
			}

			if err != nil {
				return err
			}

			v := "background-image: url(/content/" + url.QueryEscape(fileName) + ")"
			err = c.Run(ctxt, cdp.SetAttributeValue("#container", "style", v, cdp.ByID))

			return err
		}
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
	if p.api.test == "" {
		b := []byte(cmd)
		if cmd == "q" {
			p.quitting = true
		}
		_, err = p.pipeIn.Write(b)
	}

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

		err := p.Start(p.playlist.Items[i].Name(), position)
		if err != nil {
			m := &resMessage{Success: false, Message: "Error trying to start video: " + err.Error()}
			log.Println(m.Message)
			json.NewEncoder(w).Encode(m)
			return
		}

		p.playlist.Current = p.playlist.Items[i]

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

		err := p.SendCommand(cmd)
		if err != nil {
			m := &resMessage{Success: false, Message: "Error trying to execute command: " + err.Error()}
			log.Println(m.Message)
			json.NewEncoder(w).Encode(m)
			return
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

// Scan the folder for new files every time the page reloads
func (p *Player) handleControl(w http.ResponseWriter, r *http.Request) {
	err := p.playlist.fromFolder(p.conf.Directory)

	if err != nil {
		log.Println("Error tring to read files from directory: ", err)
		t := template.Must(template.ParseFiles("templates/error.html"))
		err := t.Execute(w, err)
		if err != nil {
			log.Println("Error trying to render error page. #fail.", err)
		}
		return
	}

	tempControl := templateHandler{
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
		for _, file := range p.playlist.Items {
			log.Println(file.Name())
		}
	}
	tempControl.templ = template.Must(template.ParseFiles(filepath.Join("templates", tempControl.filename)))

	err = tempControl.templ.Execute(w, tempControl.data)
	if err != nil {
		log.Println("Error rendering control page: ", err)
	}
}

func (p *Player) handleViewer(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	th := templateHandler{
		filename: "viewer.html",
		data: map[string]interface{}{
			"img": "/content/" + q.Get("img"),
		},
	}

	th.ServeHTTP(w, r)
}

// next starts the next item in the playlist
func (p *Player) next() error {
	n := p.playlist.getNext()
	err := p.Start(n.Name(), time.Duration(0))

	if err == nil {
		p.playlist.Current = n
	}

	return err
}

func (p *Player) previous() error {
	n := p.playlist.getPrevious()
	err := p.Start(n.Name(), time.Duration(0))

	if err == nil {
		p.playlist.Current = n
	}

	return err
}

func (c *controller) handlerWebsocket(w http.ResponseWriter, r *http.Request) {
	var err error
	c.socket, err = upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal("ServeHTTP:", err)
		return
	}

	go c.write()
	// we can't start the read() method on a separate goroutine, or this function would return and stop serving the websocket connections
	// we need the infinite loop in the read function to block operations and keep this function alive
	c.read()
}

func (c *controller) write() {
	defer c.socket.Close()

	// this loop keeps running as long as the channel is open.
	for msg := range c.send {
		err := c.socket.WriteJSON(msg)
		if err != nil {
			log.Println("Error trying to write JSON to the socket: ", err)
			// this probably means that the connection is broken,
			// so close the channel and return out of the loop.
			close(c.send)
			return
		}
	}
}

// read reads the messages from the socket
func (c *controller) read() {
	defer c.socket.Close()

	for {
		var msg reqMessage
		err := c.socket.ReadJSON(&msg)
		if err != nil {
			log.Println("Error trying to read the JSON from the socket: ", err)
			// this probably means that the connection is broken,
			// so close the channel and return out of the loop.
			close(c.send)
			return
		}

		// ignore socket messages for now.
		// TODO: handle socket messages.
		log.Println("socket message received: ", msg)
	}
}

func (p *Player) remoteListen(device *evdev.InputDevice) {
	errCount := 0
	commands := map[string]string{
		"KEY_HOME":         "",
		"KEY_INFO":         "",
		"KEY_UP":           "",
		"KEY_DOWN":         "",
		"KEY_LEFT":         "",
		"KEY_RIGHT":        "",
		"KEY_ENTER":        "",
		"KEY_BACK":         "",
		"KEY_CONTEXT_MENU": "",
		"KEY_PLAYPAUSE":    "pauseResume",
		"KEY_STOP":         "quit",
		"KEY_REWIND":       "seekBack30",
		"KEY_FASTFORWARD":  "seekForward30",
	}
	directions := []string{"UP", "DOWN", "HOLD"}

	for {
		events, err := device.Read()
		if err != nil {
			log.Println("Error trying to read device event:", err)
			errCount++
			if errCount > 10 {
				log.Println("Too many errors trying to read the device, breaking out")
				break
			}
		}
		for _, event := range events {
			code := int(event.Code)

			// ignore any events that are not key presses
			if event.Type != evdev.EV_KEY {
				continue
			}

			value, ok := evdev.KEY[code]
			// ignore UP and HOLD
			if !ok || directions[event.Value] != "DOWN" {
				continue
			}
			if p.api.debug {
				log.Println("Key:", value, directions[event.Value])
			}

			if value == "KEY_LEFT" {
				err := p.previous()
				if err != nil {
					log.Println("Error trying to go to previous item from remote.\n", err)
				}
				continue
			}

			if value == "KEY_RIGHT" {
				err := p.next()
				if err != nil {
					log.Println("Error trying to go to next item from remote.\n", err)
				}
				continue
			}

			c, ok := commands[value]
			// ignore empty commands, they are not supported yet
			if !ok || c == "" {
				continue
			}
			if p.api.debug {
				log.Println("command used:", c)
			}

			// don't send the command if we're only testing. This will only work on the Pi's
			if p.api.test != "" {
				log.Println("only testing, nothing was actuall sent.")
				continue
			}

			if p.api.debug {
				log.Println("not testing, sending command...")
			}
			err := p.SendCommand(c)
			if err != nil {
				log.Println("Error sending command from remote event:", err)
			}
		}
	}
}
