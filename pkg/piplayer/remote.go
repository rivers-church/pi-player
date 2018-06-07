package piplayer

import (
	"log"

	"github.com/17xande/keylogger"
)

type remote struct {
	Name    string
	Vendor  uint16
	Product uint16
}

var directions = []string{"UP", "DOWN", "HOLD"}

var remoteCommands = map[string]string{
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

func remoteRead(p *Player, cie chan keylogger.InputEvent) {
	var ie keylogger.InputEvent

	if p.api.debug {
		log.Println("starting remote read for this device")
	}

	for {
		ie = <-cie
		// ignore events that are not EV_KEY events that are KEY_DOWN presses
		if ie.Type != keylogger.EventTypes["EV_KEY"] || directions[ie.Value] != "DOWN" {
			continue
		}

		key := ie.KeyString()

		if p.api.debug {
			log.Println("Key:", key, directions[ie.Value])
			log.Println("Sending keypress to page and nothing else.")
		}

		msg := resMessage{
			Event:   "keyDown",
			Message: key,
		}
		p.Connection.send <- msg

		// if p.status == statusLive {
		// 	handleRemoteLive(p, key)
		// } else if p.status == statusMenu {
		// 	handleRemoteMenu(p, key)
		// }
	}
}

func handleRemoteLive(p *Player, key string) {
	if p.api.debug {
		log.Println("handling remote input for live media")
	}

	switch key {
	case "KEY_LEFT":
		err := p.previous()
		if err != nil {
			log.Println("Error trying to go to previous item from remote.\n", err)
		}
	case "KEY_RIGHT":
		err := p.next()
		if err != nil {
			log.Println("Error trying to go to next item from remote.\n", err)
		}
	case "KEY_HOME":
		err := p.home()
		if err != nil {
			log.Println("Error trying to go to HOME menu from remote.\n", err)
		} else {
			p.status = statusMenu
		}
	default:
		c, ok := remoteCommands[key]
		// ignore empty commands, they are not supported yet
		if !ok || c == "" {
			return
		}

		// don't send the command if we're only testing. This will only work on the Pi's
		if p.api.test != "" {
			log.Println("only testing, the following command was not sent:\n.", c)
			return
		}

		if p.api.debug {
			log.Println("not testing, sending command...", c)
		}
		if c == "quit" {
			if p.api.debug {
				log.Println("setting p.quitting to true because of stop pressed")
			}
			p.quitting = true
			p.quit = make(chan error)
		}
		err := p.SendCommand(c)
		if err != nil {
			log.Println("Error sending command from remote event:", err)
		}
		if c == "quit" {
			err := <-p.quit
			if err != nil {
				log.Println("Error trying to stop video from remote")
			}
			close(p.quit)
		}
	}

}

func handleRemoteMenu(p *Player, key string) {
	if p.api.debug {
		log.Println("handling remote input for menu")
	}

	switch {
	case key == "KEY_HOME":
		err := p.home()
		if err != nil {
			log.Println("Error trying to go to HOME menu from remote.\n", err)
		}
	case (key == "KEY_UP" || key == "KEY_DOWN" || key == "KEYPLAYPAUSE" || key == "KEY_ENTER"):
		msg := resMessage{
			Event:   "keyDown",
			Message: key,
		}
		p.Connection.send <- msg
	default:
		c, ok := remoteCommands[key]
		// ignore empty commands, they are not supported yet
		if !ok || c == "" {
			return
		}
	}
}
