package piplayer

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"path"
)

type playlist struct {
	Name    string
	Items   []Item
	Current *Item
}

// Handles requests to the playlist api
func (p *playlist) handleAPI(api *APIHandler, w http.ResponseWriter, h *http.Request) {
	var m *resMessage
	if api.message.Method == "getCurrent" {
		if p.Current != nil {
			m = &resMessage{
				Success: true,
				Event:   "current",
				Message: p.Current.Name(),
			}
		} else {
			m = &resMessage{
				Success: true,
				Event:   "noCurrent",
			}
		}
	} else if api.message.Method == "getItems" {
		m = &resMessage{
			Success: true,
			Event:   "items",
			Message: p.itemNames(),
		}
	}

	if api.debug {
		log.Println("sending current item.\n", m)
	}
	json.NewEncoder(w).Encode(m)
}

func (p *playlist) fromFolder(folderPath string) error {
	// remove all items from the current playlist if there are any
	p.Items = []Item{}

	// read files from a certain folder into a playlist
	files, err := ioutil.ReadDir(folderPath)
	if err != nil {
		return errors.New("can't read folder for items: " + err.Error())
	}

	// filter out all files except for supported ones
	for _, file := range files {
		e := path.Ext(file.Name())
		if e == ".mp4" || e == ".jpg" || e == ".jpeg" || e == ".png" || e == ".html" {
			p.Items = append(p.Items, Item{Visual: file})
		}
	}

	// scan for .mp3 files to see if any need to be attached to image files
	for _, file := range files {
		e := path.Ext(file.Name())
		if e == ".mp3" {
			audioNoExt := file.Name()[0 : len(file.Name())-len(e)]
			for i, item := range p.Items {
				visual := item.Visual.Name()
				visualNoExt := visual[0 : len(visual)-len(path.Ext(visual))]
				if audioNoExt == visualNoExt {
					p.Items[i].Audio = file
					break
				}
			}
		}
	}

	return nil
}

func (p *playlist) getIndex(fileName string) int {
	for i, item := range p.Items {
		if item.Name() == fileName {
			return i
		}
	}

	return -1
}

func (p *playlist) getNext() (*Item, error) {
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

func (p *playlist) getPrevious() (*Item, error) {
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

func (p *playlist) itemNames() []string {
	var res []string

	for _, item := range p.Items {
		res = append(res, item.Name())
	}

	return res
}
