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
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// Playlist stores the media items that can be played.
//
// Items and Current are rebuilt whenever the media directory is rescanned,
// which happens on every control and viewer page load, so mu guards them
// against the handlers reading them at the same time.
type Playlist struct {
	mu      sync.RWMutex
	Name    string
	Items   []Item
	Current *Item
	watcher *fsnotify.Watcher
}

// PlaylistView is a snapshot of a playlist, safe to hand to a template while
// the playlist itself is being rescanned.
type PlaylistView struct {
	Name    string
	Items   []Item
	Current *Item
}

// snapshot copies the playlist so a template can range over it without
// holding a lock.
func (p *Playlist) snapshot() PlaylistView {
	p.mu.RLock()
	defer p.mu.RUnlock()

	view := PlaylistView{Name: p.Name, Items: slices.Clone(p.Items)}
	if p.Current != nil {
		// Point Current into the copy, not the original backing array.
		for i := range view.Items {
			if view.Items[i].Name() == p.Current.Name() {
				view.Current = &view.Items[i]
				break
			}
		}
	}
	return view
}

// currentName returns the name of the current item, if there is one.
func (p *Playlist) currentName() (string, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.Current == nil {
		return "", false
	}
	return p.Current.Name(), true
}

// setCurrent points Current at the item at index, and reports whether the
// index was in range.
func (p *Playlist) setCurrent(index int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if index < 0 || index >= len(p.Items) {
		return false
	}
	p.Current = &p.Items[index]
	return true
}

// Presentation is used to read the presentation.json file for added cues.
type Presentation struct {
	Items []ItemString
}

// NewPlaylist creates a new playlist with media in the designated folder.
// A playlist is always returned, even when the directory watcher can't be
// created: the player works without one, it just won't notice files appearing
// on its own. Callers dereference the playlist on every page load, so handing
// back nil here would panic inside a handler later.
func NewPlaylist(p *Player, dir string) (*Playlist, error) {
	pl := &Playlist{Name: dir}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return pl, fmt.Errorf("error creating watcher: %v", err)
	}
	pl.watcher = watcher

	go pl.watch(p)

	if p.conf.DebugEnabled() {
		log.Printf("starting directory watcher for dir: %s\n", dir)
	}
	if exists(dir) {
		err = pl.watcher.Add(dir)
	}
	return pl, err
}

// Handles requests to the playlist api
func (p *Playlist) handleAPI(plr *Player, msg reqMessage, w http.ResponseWriter) {
	var m resMessage
	status := http.StatusOK

	switch msg.Method {
	case "getCurrent":
		if name, ok := p.currentName(); ok {
			m = resMessage{
				Success: true,
				Event:   "current",
				Message: name,
			}
		} else {
			m = resMessage{
				Success: true,
				Event:   "noCurrent",
			}
		}
	case "setCurrent":
		if len(msg.Arguments) == 0 {
			m = resMessage{
				Success: false,
				Event:   "noArgumentSupplied",
			}
			status = http.StatusBadRequest
			break
		}

		index, err := strconv.Atoi(msg.Arguments["index"])
		if err != nil {
			log.Printf("Error converting argument to int: playlist.HandleAPI.setCurrent\n%v", err)
		}

		if err != nil || !p.setCurrent(index) {
			m = resMessage{
				Success: false,
				Event:   "argumentInvalid",
			}
			status = http.StatusBadRequest
			break
		}

		m = resMessage{
			Success: true,
			Event:   "setCurrent",
			Message: index,
		}

		// send update to the control page, if open.
		plr.ConnControl.trySend(wsMessage{
			Success: true,
			Event:   "setCurrent",
			Message: index,
		})

		if plr.api.debug {
			log.Println("set current item index to:", index)
		}
	case "getItems":
		// Rescan the configured directory, not the one this playlist happens
		// to hold: after a settings change they differ until some page load
		// resyncs them, and the viewer would be handed the old directory.
		dir := plr.conf.MediaDir()
		if err := p.fromFolder(dir); err != nil {
			log.Printf("Api call failed. Can't get items from folder %s\n%v", dir, err)
		}

		m = resMessage{
			Success: true,
			Event:   "items",
			Message: p.itemsString(),
		}
	default:
		log.Printf("API call unsupported. Ignoring:\n%v\n", msg)
		m = resMessage{
			Success: false,
			Event:   "unsupportedMethod",
			Message: "Unsupported method: " + msg.Method,
		}
		status = http.StatusNotFound
	}

	writeAPIResponse(w, status, &m)
}

// fromFolder rescans dir and replaces the playlist's items with what it finds.
// The scan happens outside the lock, so readers only ever see the old items or
// the new ones. The current item is carried over by name where it still exists.
func (p *Playlist) fromFolder(dir string) error {
	items, err := scanFolder(dir)
	if err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	currentName := ""
	if p.Current != nil {
		currentName = p.Current.Name()
	}

	p.Name = dir
	p.Items = items
	p.Current = nil
	for i := range p.Items {
		if p.Items[i].Name() == currentName {
			p.Current = &p.Items[i]
			break
		}
	}

	return nil
}

// scanFolder reads dir and returns the playable items it contains.
func scanFolder(dir string) ([]Item, error) {
	items := []Item{}

	// Read files from a certain folder into a playlist.
	if !exists(dir) {
		return nil, fmt.Errorf("fromFolder: Can't read files from directory '%s' because it does not exist", dir)
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, errors.New("fromFolder: Can't read folder for items: " + err.Error())
	}

	// Filter out all files except for supported ones.
	for _, file := range files {
		c := make(map[string]string)
		e := strings.ToLower(path.Ext(file.Name()))
		switch e {
		case ".mp4", ".webm":
			items = append(items, Item{Visual: file, Type: "video", Cues: c})
		case ".jpg", ".jpeg", ".png":
			items = append(items, Item{Visual: file, Type: "image", Cues: c})
		case ".html":
			items = append(items, Item{Visual: file, Type: "browser", Cues: c})
		}
	}

	// scan for .mp3 files to see if any need to be attached to image files
	for _, file := range files {
		e := path.Ext(file.Name())
		if e != ".mp3" && e != ".mp0" {
			continue
		}

		audioBase := file.Name()[0 : len(file.Name())-len(e)]
		for i, item := range items {
			visual := item.Visual.Name()
			visualBase := visual[0 : len(visual)-len(path.Ext(visual))]
			if audioBase == visualBase {
				switch e {
				case ".mp3":
					items[i].Audio = file
				case ".mp0":
					items[i].Cues["clear"] = "audio"
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
			return items, nil
		}

		var presentation Presentation

		if err := json.Unmarshal(data, &presentation); err != nil {
			log.Printf("Error trying to parse presentation file '%s', ignoring its cues: %v", file, err)
			return items, nil
		}

		// Loop through presentation data and attach cues to items.
		for _, presItem := range presentation.Items {
			// Create regex to match on file names.
			r, err := regexp.Compile(presItem.Visual)
			if err != nil {
				log.Printf("Could not compile regex with text '%s', comparing using visual name only.", presItem.Visual)
			}
			for _, playItem := range items {
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

	return items, nil
}

// watch for changes in the supplied directory
func (p *Playlist) watch(plr *Player) {
	if p.watcher == nil {
		return
	}
	defer p.watcher.Close()
	for {
		select {
		case event, ok := <-p.watcher.Events:
			// This means a file changed in the folder.
			if !ok {
				log.Println("issue getting file change event. Stopping watcher.")
				return
			}
			if plr.conf.DebugEnabled() {
				log.Println("file change event:", event)
			}
			// Send a message to the viewer to get new items.
			msg := wsMessage{
				Component: "playlist",
				Event:     "newItems",
				Message:   "detected file change. Get new items.",
			}
			plr.ConnControl.trySend(msg)
		case err, ok := <-p.watcher.Errors:
			if !ok {
				log.Println("issue getting file change error. Stopping watcher.")
				return
			}
			log.Println("error:", err)
		}
	}
}

func (p *Playlist) itemsString() []ItemString {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var res []ItemString

	for _, item := range p.Items {
		res = append(res, item.String())
	}

	return res
}
