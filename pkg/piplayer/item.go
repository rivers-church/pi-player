package piplayer

import "os"

// Item represents a playlist item
// it can have a visual element, an audio element, or both.
// more elements such as timers can be added later.
type Item struct {
	Audio  os.FileInfo
	Visual os.FileInfo
}

// ItemString is a simpler representation of an Item,
// where only the file name for the Audio and Visual elements are stored.
type ItemString struct {
	Audio  string
	Visual string
}

// Name returns the filename of the visual element
func (i *Item) Name() string {
	return i.Visual.Name()
}

func (i *Item) String() ItemString {
	is := ItemString{}
	if i.Audio != nil {
		is.Audio = i.Audio.Name()
	}
	if i.Visual != nil {
		is.Visual = i.Visual.Name()
	}
	return is
}
