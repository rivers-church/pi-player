package piplayer

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/17xande/keylogger"
)

type remote struct {
	Name    string
	Vendor  uint16
	Product uint16
}

var directions = []string{"UP", "DOWN", "HOLD"}

func remoteRead(ctx context.Context, p *Player) {
	for {
		if p.api.debug {
			log.Println("starting remote read for this device")
		}
		if err := Listen(ctx, p.conf.Remote.Name, p); err != nil {
			if p.api.debug {
				log.Printf("error listening to device, retrying in 3 seconds: %v\n", err)
				time.Sleep(3 * time.Second)
			}
		}
	}

}

// Listen to all the Input Devices supplied.
// Return an error if there is a problem, or if one of the devices disconnects.
func Listen(ctx context.Context, dev string, p *Player) []error {
errs := make([]error, 3)
	send := p.ConnViewer.getChanSend()
	kl := keylogger.NewKeyLogger(dev)
	if len(kl.GetDevices()) <= 0 {
		return []error{fmt.Errorf("device '%s' not found", dev)}
	}

	for _, d := range kl.GetDevices() {
		if p.api.debug {
			log.Printf("Listening to device %s\n", d.Name)
		}
	}

	cie := make(chan keylogger.InputEvent)
	cer := make(chan error)
	cwait := make(chan struct{})

	go kl.Read(ctx, cwait, cie, cer)

	for {
		select {
		case <-cwait:
			return errs
		case e, open := <-cie:
			if !open {
				errs = append(errs, fmt.Errorf("event channel closed"))
			}
			// Ignore events that are not EV_KEY events that are KEY_DOWN presses
			if e.Type != keylogger.EventTypes["EV_KEY"] || directions[e.Value] != "DOWN" {
				continue
			}
			key := e.KeyString()

			if p.api.debug {
				log.Printf("Key: %s\tValue: %s\tType: %d\n", key, directions[e.Value], e.Type)
				log.Println("Sending keypress to page and nothing else.")
			}

			msg := wsMessage{
				Component: "remote",
				Arguments: map[string]string{"keyString": key},
				Event:     "keyDown",
			}

			send <- msg

			if p.api.debug {
				log.Println("Message sent")
			}

		case err, open := <-cer:
			if !open {
				errs = append(errs, fmt.Errorf("error channel closed"))
			}
			errs = append(errs, err)
		}
	}
}
