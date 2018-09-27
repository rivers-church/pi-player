package piplayer

import (
	"os"
	"path/filepath"
)

// Item represents a playlist item
// it can have a visual element, an audio element, or both.
// more elements such as timers can be added later.
type Item struct {
	Audio  os.FileInfo
	Visual os.FileInfo
	Type   string
	Cues   map[string]string
}

// ItemString is a simpler representation of an Item,
// where only the file name for the Audio and Visual elements are stored.
type ItemString struct {
	Audio  string
	Visual string
	Type   string
	Cues   map[string]string
}

// Name returns the filename of the visual element.
func (i *Item) Name() string {
	return removeExtension(i.Visual.Name())
}

// String returns an newly created ItemString version of the Item.
func (i *Item) String() ItemString {
	is := ItemString{}
	if i.Audio != nil {
		is.Audio = i.Audio.Name()
	}
	if i.Visual != nil {
		is.Visual = i.Visual.Name()
	}

	is.Type = i.Type
	is.Cues = i.Cues
	return is
}

func removeExtension(filename string) string {
	ext := filepath.Ext(filename)
	l := len(filename) - len(ext)
	return filename[:l]
}
