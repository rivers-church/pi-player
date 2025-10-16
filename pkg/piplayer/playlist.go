package piplayer

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"net/http"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/fsnotify/fsnotify"
)

// Playlist stores the media items that can be played.
type Playlist struct {
	Name    string
	Items   []Item
	Current *Item
	watcher *fsnotify.Watcher
}

// Presentation is used to read the presentation.json file for added cues.
type Presentation struct {
	Items []ItemString
}

// NewPlaylist creates a new playlist with media in the designated folder.
func NewPlaylist(p *Player, dir string) (*Playlist, error) {
	pl := &Playlist{Name: dir}

	var err error
	pl.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("error creating watcher: %v", err)
	}

	go pl.watch(p)

	if p.conf.Debug {
		log.Printf("starting directory watcher for dir: %s\n", dir)
	}
	err = pl.watcher.Add(dir)
	return pl, err
}

// Handles requests to the playlist api
func (p *Playlist) handleAPI(plr *Player, w http.ResponseWriter, h *http.Request) {
	var m resMessage

	switch plr.api.message.Method {
	case "getCurrent":
		if p.Current != nil {
			m = resMessage{
				Success: true,
				Event:   "current",
				Message: p.Current.Name(),
			}
		} else {
			m = resMessage{
				Success: true,
				Event:   "noCurrent",
			}
		}
	case "setCurrent":
		if len(plr.api.message.Arguments) == 0 {
			m = resMessage{
				Success: false,
				Event:   "noArgumentSupplied",
			}
			break
		}

		index, err := strconv.Atoi(plr.api.message.Arguments["index"])
		if err != nil {
			log.Printf("Error converting argument to int: playlist.HandleAPI.setCurrent\n%v", err)
		}

		if err != nil || index < 0 || index >= len(p.Items) {
			m = resMessage{
				Success: false,
				Event:   "argumentInvalid",
			}
			break
		}

		p.Current = &p.Items[index]

		m = resMessage{
			Success: true,
			Event:   "setCurrent",
			Message: index,
		}

		// send update to the control page, if open.
		if plr.ConnControl.isActive() {
			m := wsMessage{
				Success: true,
				Event:   "setCurrent",
				Message: index,
			}
			send := plr.ConnControl.getChanSend()
			send <- m
		}

		if plr.api.debug {
			log.Println("set current item index to:", index)
		}
	case "getItems":
		if err := p.fromFolder(p.Name); err != nil {
			log.Printf("Api call failed. Can't get items from folder %s\n%v", p.Name, err)
		}

		m = resMessage{
			Success: true,
			Event:   "items",
			Message: p.itemsString(),
		}
	default:
		log.Printf("API call unsupported. Ignoring:\n%v\n", plr.api.message)
	}

	json.NewEncoder(w).Encode(m)
}

func (p *Playlist) fromFolder(dir string) error {
	// Remove all items from the current playlist if there are any.
	p.Items = []Item{}
	p.Name = dir

	// Read files from a certain folder into a playlist.
	if !exists(dir) {
		return fmt.Errorf("fromFolder: Can't read files from directory '%s' because it does not exist", dir)
	}
	// files, err := ioutil.ReadDir(dir)
	files, err := os.ReadDir(dir)
	if err != nil {
		return errors.New("fromFolder: Can't read folder for items: " + err.Error())
	}

	// Filter out all files except for supported ones.
	for _, file := range files {
		c := make(map[string]string)
		e := strings.ToLower(path.Ext(file.Name()))
		switch e {
		case ".mp4", ".webm":
			p.Items = append(p.Items, Item{Visual: file, Type: "video", Cues: c})
		case ".jpg", ".jpeg", ".png":
			p.Items = append(p.Items, Item{Visual: file, Type: "image", Cues: c})
		case ".html":
			p.Items = append(p.Items, Item{Visual: file, Type: "browser", Cues: c})
		}
	}

	// scan for .mp3 files to see if any need to be attached to image files
	for _, file := range files {
		e := path.Ext(file.Name())
		if e != ".mp3" && e != ".mp0" {
			continue
		}

		audioBase := file.Name()[0 : len(file.Name())-len(e)]
		for i, item := range p.Items {
			visual := item.Visual.Name()
			visualBase := visual[0 : len(visual)-len(path.Ext(visual))]
			if audioBase == visualBase {
				switch e {
				case ".mp3":
					p.Items[i].Audio = file
				case ".mp0":
					p.Items[i].Cues["clear"] = "audio"
				}
				break
			}

		}
	}

	// look for presentation file for added cues.
	file := path.Join(dir, "presentation.json")
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		data, err := os.ReadFile(file)
		if err != nil {
			log.Printf("Error trying to read presentation file '%s': %v", file, err)
			return nil
		}

		var presentation Presentation

		json.Unmarshal(data, &presentation)

		// Loop through presentation data and attach cues to items.
		for _, presItem := range presentation.Items {
			// Create regex to match on file names.
			r, err := regexp.Compile(presItem.Visual)
			if err != nil {
				log.Printf("Could not compile regex with text '%s', comparing using visual name only.", presItem.Visual)
			}
			for _, playItem := range p.Items {
				// If the regex can't compile, use the file name, otherwise use the regex.
				if err != nil && presItem.Visual == playItem.Visual.Name() {
					maps.Copy(playItem.Cues, presItem.Cues)
					break
				} else if err == nil && r.MatchString(playItem.Visual.Name()) {
					maps.Copy(playItem.Cues, presItem.Cues)
				}
			}
		}
	}

	return nil
}

// watch for changes in the supplied directory
func (p *Playlist) watch(plr *Player) {
	defer p.watcher.Close()
	send := plr.ConnControl.getChanSend()
	for {
		select {
		case event, ok := <-p.watcher.Events:
			// This means a file changed in the folder.
			if !ok {
				log.Println("issue getting file change event. Stopping watcher.")
				return
			}
			if plr.conf.Debug {
				log.Println("file change event:", event)
			}
			// Send a message to the viewer to get new items.
			msg := wsMessage{
				Component: "playlist",
				Event:     "newItems",
				Message:   "detected file change. Get new items.",
			}
			send <- msg
		case err, ok := <-p.watcher.Errors:
			if !ok {
				log.Println("issue getting file change error. Stopping watcher.")
				return
			}
			log.Println("error:", err)
		}
	}
}

// func (p *Playlist) getIndex(fileName string) int {
// 	for i, item := range p.Items {
// 		if item.Name() == fileName {
// 			return i
// 		}
// 	}

// 	return -1
// }

// func (p *Playlist) getNext() (*Item, error) {
// 	if p.Current == nil {
// 		return nil, errors.New("no current item, can't get next")
// 	}

// 	i := p.getIndex(p.Current.Name())
// 	if i == -1 {
// 		return nil, errors.New("can't find index of this item: " + p.Current.Name())
// 	}
// 	if i+1 > len(p.Items)-1 {
// 		return &p.Items[0], nil
// 	}

// 	return &p.Items[i+1], nil
// }

// func (p *Playlist) getPrevious() (*Item, error) {
// 	if p.Current == nil {
// 		return nil, errors.New("no current item, can't get previous")
// 	}

// 	i := p.getIndex(p.Current.Name())
// 	if i == -1 {
// 		return nil, errors.New("can't find index of this item: " + p.Current.Name())
// 	}
// 	if i-1 < 0 {
// 		return &p.Items[len(p.Items)-1], nil
// 	}

// 	return &p.Items[i-1], nil
// }

func (p *Playlist) itemsString() []ItemString {
	var res []ItemString

	for _, item := range p.Items {
		res = append(res, item.String())
	}

	return res
}
