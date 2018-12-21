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
	Open(file string, test string, debug bool) error
	Close() error
	Play() error
	Pause() error
	PlaybackRate(rate int) error
	Seek(seconds int) error
	Chapter(chapter int) error
	Volume(volume int) error
	AudioStream(stream int) error
	SubtitleStream(stream int) error
}
