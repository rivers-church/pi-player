package piplayer

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/17xande/keylogger"
)

type remote struct {
	Names   []string
	Vendor  uint16
	Product uint16
}

var directions = []string{"UP", "DOWN", "HOLD"}

// remoteRetryDelay is how long to wait before looking for the remote again
// after listening to it fails, typically because it isn't plugged in.
const remoteRetryDelay = 3 * time.Second

// remoteRead listens to the remote control until ctx is cancelled, retrying
// whenever the device goes away.
func remoteRead(ctx context.Context, p *Player) {
	for {
		logger.Debug("starting remote read for this device")
		if err := Listen(ctx, p.conf.Remote.Names, p); err != nil {
			logger.Error("listening to remote failed, retrying", "retryIn", remoteRetryDelay, "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(remoteRetryDelay):
		}
	}
}

// Listen to all the Input Devices supplied.
// Return an error if there is a problem, or if one of the devices disconnects.
func Listen(ctx context.Context, devs []string, p *Player) error {
	kl := keylogger.NewKeyLogger(devs)
	if len(kl.GetDevices()) <= 0 {
		return fmt.Errorf("device '%s' not found", devs)
	}

	for _, d := range kl.GetDevices() {
		logger.Debug("listening to device", "device", d.Name)
	}

	cie := make(chan keylogger.InputEvent)
	cer := make(chan error)
	cwait := make(chan struct{})

	go kl.Read(ctx, cwait, cie, cer)

	var errs []error
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-cwait:
			return errors.Join(errs...)
		case e, open := <-cie:
			if !open {
				errs = append(errs, errors.New("event channel closed"))
				return errors.Join(errs...)
			}
			// Ignore events that are not EV_KEY events that are KEY_DOWN presses.
			// e.Value comes from the device, so it can be outside the range of
			// directions we know about.
			if e.Type != keylogger.EventTypes["EV_KEY"] || e.Value < 0 || int(e.Value) >= len(directions) || directions[e.Value] != "DOWN" {
				continue
			}
			key := e.KeyString()

			logger.Debug("remote keypress", "key", key, "value", directions[e.Value], "type", e.Type)

			msg := wsMessage{
				Component: "remote",
				Arguments: map[string]string{"keyString": key},
				Event:     "keyDown",
			}

			p.ConnViewer.trySend(msg)

		case err, open := <-cer:
			if !open {
				errs = append(errs, errors.New("error channel closed"))
				return errors.Join(errs...)
			}
			errs = append(errs, err)
		}
	}
}
