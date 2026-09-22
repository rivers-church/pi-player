package piplayer

import (
	"encoding/json"
	"errors"
	"fmt"
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

// playlistView is a snapshot of a playlist, safe to hand to a template while
// the playlist itself is being rescanned.
type playlistView struct {
	Items   []Item
	Current *Item
}

// snapshot copies the playlist so a template can range over it without
// holding a lock.
func (p *Playlist) snapshot() playlistView {
	p.mu.RLock()
	defer p.mu.RUnlock()

	view := playlistView{Items: slices.Clone(p.Items)}
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

// presentation is used to read the presentation.json file for added cues.
type presentation struct {
	Items []itemString
}

// newPlaylist creates a new playlist with media in the designated folder.
// A playlist is always returned, even when the directory watcher can't be
// created: the player works without one, it just won't notice files appearing
// on its own. Callers dereference the playlist on every page load, so handing
// back nil here would panic inside a handler later.
func newPlaylist(dir string, control notifier) (*Playlist, error) {
	pl := &Playlist{Name: dir}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return pl, fmt.Errorf("error creating watcher: %v", err)
	}
	pl.watcher = watcher

	go pl.watch(control)

	logger.Debug("starting directory watcher", "dir", dir)
	if exists(dir) {
		err = pl.watcher.Add(dir)
	}
	return pl, err
}

// Handles requests to the playlist api
func (p *Playlist) handleAPI(msg reqMessage, w http.ResponseWriter, dir string, control notifier) {
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
			logger.Warn("setCurrent got an index that isn't a number", "index", msg.Arguments["index"], "error", err)
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
		control.trySend(wsMessage{
			Success: true,
			Event:   "setCurrent",
			Message: index,
		})

		logger.Debug("set current item", "index", index)
	case "getItems":
		// Rescan the directory the config points at, not the one this playlist
		// happens to hold: after a settings change they differ until some page
		// load resyncs them, and the viewer would be handed the old directory.
		if err := p.fromFolder(dir); err != nil {
			logger.Error("could not read items from the media directory", "dir", dir, "error", err)
		}

		m = resMessage{
			Success: true,
			Event:   "items",
			Message: p.itemsString(),
		}
	default:
		logger.Warn("unsupported playlist API call", "method", msg.Method)
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
			logger.Error("could not read the presentation file", "file", file, "error", err)
			return items, nil
		}

		var presentation presentation

		if err := json.Unmarshal(data, &presentation); err != nil {
			logger.Error("could not parse the presentation file, ignoring its cues", "file", file, "error", err)
			return items, nil
		}

		// Loop through presentation data and attach cues to items.
		for _, presItem := range presentation.Items {
			// Create regex to match on file names.
			r, err := regexp.Compile(presItem.Visual)
			if err != nil {
				logger.Warn("presentation pattern is not a valid regex, matching on the file name instead", "pattern", presItem.Visual)
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
func (p *Playlist) watch(control notifier) {
	if p.watcher == nil {
		return
	}
	defer p.watcher.Close()
	for {
		select {
		case event, ok := <-p.watcher.Events:
			// This means a file changed in the folder.
			if !ok {
				logger.Info("file change channel closed, stopping the directory watcher")
				return
			}
			logger.Debug("file change", "event", event.String())
			// Send a message to the viewer to get new items.
			msg := wsMessage{
				Component: "playlist",
				Event:     "newItems",
				Message:   "detected file change. Get new items.",
			}
			control.trySend(msg)
		case err, ok := <-p.watcher.Errors:
			if !ok {
				logger.Info("watcher error channel closed, stopping the directory watcher")
				return
			}
			logger.Error("directory watcher error", "error", err)
		}
	}
}

func (p *Playlist) itemsString() []itemString {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var res []itemString

	for _, item := range p.Items {
		res = append(res, item.String())
	}

	return res
}
