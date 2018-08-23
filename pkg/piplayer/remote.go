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
		p.ConnViewer.send <- msg
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
		log.Println("Key not supported: ", key)
	}

}
