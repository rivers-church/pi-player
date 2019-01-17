package piplayer

import (
	"errors"
	"os"
	"os/exec"
)

// Chrome represents Google Chrome as the video stream playback software.
// It implements the streamer interface.
type Chrome struct {
	Supports    []string
	status      int
	closing     chan error
	cmd         *exec.Cmd
	ConnViewer  ConnectionWS
	ConnControl ConnectionWS
}

const (
	defaultProgram = "chromium-browser"
	viewerPage     = "http://localhost:8080/viewer"
)

var defaultFlags = []string{
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
	// Experimental gpu enabling flags for potentially higher video playback performance.
	// Use with caution, these are not stable.
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

// Open starts Chrome in the relevant page with the relevant flags.
func (c *Chrome) Open(file string, status chan string, test string, debug bool) error {
	// If Chrome is already running ignore and return error
	if c.status == statusStopped {
		return errors.New("cannot start Chrome, it's already running")
	}

	flags := defaultFlags
	program := defaultProgram

	if test != "" {
		flags = []string{
			"--incognito",
			"--remote-debugging-port=9222",
		}
	}

	switch test {
	case "linux":
		program = "google-chrome"
	case "mac":
		program = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
	default:
	}

	flags = append(flags, viewerPage)
	c.cmd = exec.Command(program, flags...)
	if test != "" {
		c.cmd.Env = []string{"DISPLAY=:0.0"}
	}

	c.cmd.Stdin = os.Stdin
	c.cmd.Stderr = os.Stderr
	if debug {
		c.cmd.Stdout = os.Stdout
	}

	if err := c.cmd.Start(); err != nil {
		return err
	}

	c.status = statusStarting
	// TODO: handle graceful shutdown properly with context
	// ctxt, cancel := context.WithCancel(context.Background())

	return nil
}

// HandleRes checks the WebSocket response for errors.
func handleRes(res wsMessage) error {
	if !res.Success {
		resMsg := res.Message.(string)
		return errors.New(resMsg)
	}
	return nil
}

// Close closes Google Chrome.
func (c *Chrome) Close() error {
	// TODO: everything
	return nil
}

// Play sends a play command.
func (c *Chrome) Play() error {
	msg := wsMessage{
		Component: "player",
		Method:    "play",
	}

	// TODO: create convenience method that sends and waits?
	c.ConnViewer.send <- msg
	// Wait for response
	res := <-c.ConnViewer.receive
	return handleRes(res)
}

// Pause sends a pause command.
func (c *Chrome) Pause() error {
	msg := wsMessage{
		Component: "player",
		Method:    "pause",
	}

	c.ConnViewer.send <- msg
	res := <-c.ConnViewer.receive
	return handleRes(res)
}

// PlaybackRate sets the video playback rate.
func (c *Chrome) PlaybackRate(rate int) error {
	msg := wsMessage{
		Component: "player",
		Method:    "playbackRate",
		Arguments: map[string]string{
			"rate": string(rate),
		},
	}

	c.ConnViewer.send <- msg
	res := <-c.ConnViewer.receive
	return handleRes(res)
}

// Seek seeks to a specific time in a video.
func (c *Chrome) Seek(seconds int) error {
	msg := wsMessage{
		Component: "player",
		Method:    "seek",
		Arguments: map[string]string{
			"seconds": string(seconds),
		},
	}

	c.ConnViewer.send <- msg
	res := <-c.ConnViewer.receive
	return handleRes(res)
}

// Chapter seeks to a specific chapter in the video.
func (c *Chrome) Chapter(chp int) error {
	msg := wsMessage{
		Component: "player",
		Method:    "chapter",
		Arguments: map[string]string{
			"chapter": string(chp),
		},
	}

	c.ConnViewer.send <- msg
	res := <-c.ConnViewer.receive
	return handleRes(res)
}

// Volume sets the video volume.
func (c *Chrome) Volume(v int) error {
	msg := wsMessage{
		Component: "player",
		Method:    "volume",
		Arguments: map[string]string{
			"rate": string(v),
		},
	}

	c.ConnViewer.send <- msg
	res := <-c.ConnViewer.receive
	return handleRes(res)
}

// AudioStream sets the video audio stream.
func (c *Chrome) AudioStream(a int) error {
	msg := wsMessage{
		Component: "player",
		Method:    "audioStream",
		Arguments: map[string]string{
			"rate": string(a),
		},
	}

	c.ConnViewer.send <- msg
	res := <-c.ConnViewer.receive
	return handleRes(res)
}

// SubtitleStream sets the video subtitle stream.
func (c *Chrome) SubtitleStream(s int) error {
	msg := wsMessage{
		Component: "player",
		Method:    "subtitleStream",
		Arguments: map[string]string{
			"rate": string(s),
		},
	}

	c.ConnViewer.send <- msg
	res := <-c.ConnViewer.receive
	return handleRes(res)
}
