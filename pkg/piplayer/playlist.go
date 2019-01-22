package piplayer

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strconv"
)

// Playlist stores the media items that can be played.
type Playlist struct {
	Name    string
	Items   []Item
	Current *Item
}

// Presentation is used to read the presentation.json file for added cues.
type Presentation struct {
	Items []ItemString
}

// NewPlaylist creates a new playlist with media in the designated folder.
func NewPlaylist(p *Player, dir string) (Playlist, error) {
	pl := Playlist{Name: dir}
	err := pl.fromFolder(p, dir)
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
		if plr.api.message.Arguments == nil || len(plr.api.message.Arguments) == 0 {
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
		if plr.ConnControl.active {
			m := wsMessage{
				Success: true,
				Event:   "setCurrent",
				Message: index,
			}
			plr.ConnControl.send <- m
		}

		if plr.api.debug {
			log.Println("set current item index to:", index)
		}
	case "getItems":
		if err := p.fromFolder(plr, p.Name); err != nil {
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

func (p *Playlist) fromFolder(plr *Player, folderPath string) error {
	// Remove all items from the current playlist if there are any.
	p.Items = []Item{}
	p.Name = folderPath

	// Read files from a certain folder into a playlist.
	files, err := ioutil.ReadDir(folderPath)
	if err != nil {
		return errors.New("can't read folder for items: " + err.Error())
	}

	// Filter out all files except for supported ones.
	for _, file := range files {
		c := make(map[string]string)
		e := path.Ext(file.Name())
		if e == ".mp4" || e == ".webm" {
			p.Items = append(p.Items, Item{Visual: file, Type: "video", Cues: c})
		} else if e == ".jpg" || e == ".jpeg" || e == ".png" {
			p.Items = append(p.Items, Item{Visual: file, Type: "image", Cues: c})
		} else if e == ".html" {
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
				if e == ".mp3" {
					p.Items[i].Audio = file
				} else if e == ".mp0" {
					p.Items[i].Cues["clear"] = "audio"
				}
				break
			}

		}
	}

	// look for presentation file for added cues.
	file := path.Join(folderPath, "presentation.json")
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		data, err := ioutil.ReadFile(file)
		if err != nil {
			log.Printf("Error trying to read presentation file '%s': %v", file, err)
			return nil
		}

		var presentation Presentation

		json.Unmarshal(data, &presentation)

		// Loop through presentation data and attach cues to items.
		for _, presItem := range presentation.Items {
			for _, playItem := range p.Items {
				if presItem.Visual == playItem.Visual.Name() {
					for k, v := range presItem.Cues {
						playItem.Cues[k] = v
					}
					break
				}
			}
		}
	}

	return nil
}

func (p *Playlist) getIndex(fileName string) int {
	for i, item := range p.Items {
		if item.Name() == fileName {
			return i
		}
	}

	return -1
}

func (p *Playlist) getNext() (*Item, error) {
	if p.Current == nil {
		return nil, errors.New("no current item, can't get next")
	}

	i := p.getIndex(p.Current.Name())
	if i == -1 {
		return nil, errors.New("can't find index of this item: " + p.Current.Name())
	}
	if i+1 > len(p.Items)-1 {
		return &p.Items[0], nil
	}

	return &p.Items[i+1], nil
}

func (p *Playlist) getPrevious() (*Item, error) {
	if p.Current == nil {
		return nil, errors.New("no current item, can't get previous")
	}

	i := p.getIndex(p.Current.Name())
	if i == -1 {
		return nil, errors.New("can't find index of this item: " + p.Current.Name())
	}
	if i-1 < 0 {
		return &p.Items[len(p.Items)-1], nil
	}

	return &p.Items[i-1], nil
}

func (p *Playlist) itemsString() []ItemString {
	var res []ItemString

	for _, item := range p.Items {
		res = append(res, item.String())
	}

	return res
}
