package piplayer

import (
	"io/fs"
	"path/filepath"
)

// Item represents a playlist item
// it can have a visual element, an audio element, or both.
// more elements such as timers can be added later.
type Item struct {
	Audio  fs.DirEntry
	Visual fs.DirEntry
	Type   string
	Cues   map[string]string
	// thumb is the URL of this item's thumbnail, worked out when the folder is
	// scanned and the file information is already in hand. Unexported with a
	// method to read it: the control template has to reach it, and nothing
	// outside the scan should be able to set it.
	thumb string
}

// itemString is a simpler representation of an Item, where only the file name
// for the Audio and Visual elements are stored. This is the shape the pages
// receive from the API.
//
// It is deliberately not the shape presentation.json is parsed into - see
// presentationItem. That file comes off the media share, and Thumb is computed
// by the player from the file it scanned, not taken from anyone who can write
// to the share.
type itemString struct {
	Audio  string
	Visual string
	Type   string
	Cues   map[string]string
	Thumb  string `json:",omitempty"`
}

// Name returns the filename of the visual element.
func (i *Item) Name() string {
	return removeExtension(i.Visual.Name())
}

// ThumbURL is the URL of this item's thumbnail, or "" for an item that has
// none - a page with nothing here falls back to the item's type icon.
func (i *Item) ThumbURL() string {
	return i.thumb
}

// String returns an newly created itemString version of the Item.
func (i *Item) String() itemString {
	is := itemString{}
	if i.Audio != nil {
		is.Audio = i.Audio.Name()
	}
	if i.Visual != nil {
		is.Visual = i.Visual.Name()
	}

	is.Type = i.Type
	is.Cues = i.Cues
	is.Thumb = i.thumb
	return is
}

func removeExtension(filename string) string {
	ext := filepath.Ext(filename)
	l := len(filename) - len(ext)
	return filename[:l]
}
