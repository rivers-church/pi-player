package piplayer

import (
	"errors"
	"io"
	"os"
	"os/exec"
)

const (
	statusStopped  = 0
	statusStarting = 1
	statusPaused   = 2
	statusPlaying  = 3
	statusClosing  = 4
	statusError    = -1
)

// OMXPlayer uses OMXPlayer to stream H.264 files to the screen.
// It implements the Streamer interface.
type OMXPlayer struct {
	Supports     []string
	status       int
	closing      chan error
	cmd          *exec.Cmd
	cmdStdinPipe io.WriteCloser
	playbackRate int
}

// Open starts a video file in the OMXPlayer streamer.
func (o *OMXPlayer) Open(filename string, audioOutput string, loop bool, debug bool, player *Player) error {
	o.status = statusStarting

	// Attempt to close OMXPlayer in case it's already running.
	if err := o.Close(); err != nil {
		return err
	}

	flags := []string{
		"-b",
		"-o", audioOutput,
		filename,
	}

	if loop {
		flags = append(flags, "--loop")
	}

	var err error
	o.cmd = exec.Command("omxplayer", flags...)
	o.cmdStdinPipe, err = o.cmd.StdinPipe()
	if err != nil {
		return err
	}

	if debug {
		o.cmd.Stdout = os.Stdout
	}
	o.cmd.Stderr = os.Stderr

	err = o.cmd.Start()
	if err != nil {
		o.status = statusError
		return err
	}

	o.status = statusPlaying
	o.playbackRate = 1

	// Listen for when OMX player ends in a new goroutine
	go o.wait(player)

	return nil
}

// wait waits for OMXPlayer to end so it can clean things up.
func (o *OMXPlayer) wait(player *Player) error {
	// Block till the command/process is finished.
	err := o.cmd.Wait()
	prevStatus := o.status
	o.status = statusStopped

	if prevStatus == statusClosing {
		o.closing <- err
	} else {
		// Start the next item.
		err = player.Next()
	}

	return err
}

// Close stops the video and closes the streamer process.
func (o *OMXPlayer) Close() error {
	if o.status != statusPlaying {
		return nil
	}

	o.status = statusClosing
	o.closing = make(chan error)
	defer close(o.closing)

	o.pipe("q")
	// Block till OMXPlayer exits
	err := <-o.closing
	// Ignore exit status 3
	if err != nil && err.Error() != "exit status 3" {
		return err
	}

	return nil
}

// pipe Pipes a message to the command
func (o *OMXPlayer) pipe(message string) error {
	_, err := o.cmdStdinPipe.Write([]byte(message))
	return err
}

// Play resumes video playback.
func (o *OMXPlayer) Play() error {
	// Ignore if the player isn't in a paused state
	if o.status != statusPaused {
		return nil
	}

	return o.pipe("p")
}

// Pause pauses the video playback.
func (o *OMXPlayer) Pause() error {
	// Ignore if the player isn't in a playing state.
	if o.status != statusPlaying {
		return nil
	}

	return o.pipe("p")
}

// PlaybackRate sets the video playback speed.
func (o *OMXPlayer) PlaybackRate(speed int) error {
	if o.status != statusPlaying && o.status != statusPaused {
		return errors.New("can't set the PlaybackRate. Video not running")
	}

	for speed < o.playbackRate {
		o.pipe("2")
		o.playbackRate--
	}

	for speed > o.playbackRate {
		o.pipe("1")
		o.playbackRate++
	}

	return nil
}

// Seek seeks the video to a certain time.
func (o *OMXPlayer) Seek(direction int) error {
	if o.status != statusPlaying && o.status != statusPaused {
		return errors.New("can't seek the video. Video not running")
	}
	switch direction {
	case -2:
		o.pipe("\x1b[B")
	case -1:
		o.pipe("\x1b[D")
	case 1:
		o.pipe("\x1b[C")
	case 2:
		o.pipe("\x1b[A")
	}
	return nil
}

// Chapter brings the video to the specified chatper.
func (o *OMXPlayer) Chapter(index int) error {
	// TODO: everything
	return nil
}

// Volume sets the video volume.
func (o *OMXPlayer) Volume(vol float64) error {
	// TODO: everything
	return nil
}

// AudioStream sets the specified video audio stream.
func (o *OMXPlayer) AudioStream(index int) error {
	// TODO: everything
	return nil
}

// SubtitleStream sets the subtitle stream.
func (o *OMXPlayer) SubtitleStream(index int) error {
	// TODO: everything
	return nil
}
