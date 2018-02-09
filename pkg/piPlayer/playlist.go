package piPlayer

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
)

type playlist struct {
	Name    string
	Items   []os.FileInfo
	Current os.FileInfo
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
	p.Items = []os.FileInfo{}

	// read files from a certain folder into a playlist
	files, err := ioutil.ReadDir(folderPath)
	if err != nil {
		return errors.New("can't read folder for videos: " + err.Error())
	}

	// filter out all files except for supported ones
	for _, file := range files {
		e := path.Ext(file.Name())
		if e == ".mp4" || e == ".jpg" || e == ".jpeg" || e == ".png" || e == ".html" {
			p.Items = append(p.Items, file)
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

func (p *playlist) getNext() os.FileInfo {
	i := p.getIndex(p.Current.Name())
	if i == -1 {
		return nil
	}
	if i+1 > len(p.Items)-1 {
		return p.Items[0]
	}

	return p.Items[i+1]
}

func (p *playlist) getPrevious() os.FileInfo {
	i := p.getIndex(p.Current.Name())
	if i == -1 {
		return nil
	}
	if i-1 < 0 {
		return p.Items[len(p.Items)-1]
	}

	return p.Items[i-1]
}

func (p *playlist) itemNames() []string {
	var res []string

	for _, item := range p.Items {
		res = append(res, item.Name())
	}

	return res
}
