package piplayer

import "os"

// Item represents a playlist item
// it can have a visual element, an audio element, or both.
// more elements such as timers can be added later.
type Item struct {
	Visual os.FileInfo
	Audio  os.FileInfo
}

// Name returns the filename of the visual element
func (i *Item) Name() string {
	return i.Visual.Name()
}
