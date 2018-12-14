package piplayer

import "time"

// Streamer represents a program that can decode a media file for display.
// It is the code that actually plays a media file.
type Streamer interface {
	Open(string) error
	Close() error
	Play() error
	Pause() error
	Speed(float64) error
	Seek(time.Duration) error
	Chapter(int) error
	Volume(float64) error
	AudioStream(int) error
	SubtitleStream(int) error
}

// OMXStreamer uses OMXPlayer to stream H.264 files to the screen.
type OMXStreamer struct {
	Supports []string
}

// Open starts a video file in the OMXPlayer streamer.
func (o *OMXStreamer) Open(filename string) error {
	// TODO: everything
	return nil
}

// Close stops the video and quits the streamer process.
func (o *OMXStreamer) Close() error {
	// TODO: everything
	return nil
}

// Play resumes video playback.
func (o *OMXStreamer) Play() error {
	// TODO: everything
	return nil
}
