package piplayer

// Chrome represents Google Chrome as the video stream playback software.
// It implements the streamer interface.
type Chrome struct {
}

// Open starts Chrome in the relevant page with the relevant flags.
func Open(file string) error {
	// TODO: everything
	return nil
}

// Close closes Google Chrome.
func Close() error {
	// TODO: everything
	return nil
}

// Play sends a play command.
func Play() error {
	// TODO: everything
	return nil
}

// Pause sends a pause command.
func Pause() error {
	// TODO: everything
	return nil
}

// PlaybackRate sets the video playback rate.
func PlaybackRate() error {
	// TODO: everything
	return nil
}

// Seek seeks to a specific time in a video.
func Seek(seconds int) error {
	// TODO: everything
	return nil
}

// Chapter seeks to a specific chapter in the video.
func Chapter(c int) error {
	// TODO: everything
	return nil
}

// Volume sets the video volume.
func Volume(v int) error {
	// TODO: everything
	return nil
}

// AudioStream sets the video audio stream.
func AudioStream(a int) error {
	// TODO: everything
	return nil
}

// SubtitleStream sets the video subtitle stream.
func SubtitleStream(s int) error {
	// TODO: everything
	return nil
}
