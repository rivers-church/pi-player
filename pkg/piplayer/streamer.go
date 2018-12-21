package piplayer

const (
	statusStopped  = 0
	statusStarting = 1
	statusPaused   = 2
	statusPlaying  = 3
	statusClosing  = 4
	statusError    = -1
)

// Streamer represents a program that can decode a media file for display.
// It is the code that actually plays a media file.
// Eg: OMXPlayer, VLC and Chrome.
type Streamer interface {
	Open(string) error
	Close() error
	Play() error
	Pause() error
	PlaybackRate(int) error
	Seek(int) error
	Chapter(int) error
	Volume(int) error
	AudioStream(int) error
	SubtitleStream(int) error
}
