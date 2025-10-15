package piplayer

import (
	"encoding/json"
	"net/url"
	"os"
)

// sURL is copy of url.URL but with its own JSON marshalling and unmarshalling methods.
type sURL struct {
	*url.URL
}

// Mount holds the details of the network or usb mount location.
type mount struct {
	Dir string `json:"-"`
	URL sURL
}

func (u sURL) MarshalJSON() ([]byte, error) {
	if u.URL == nil {
		return make([]byte, 0), nil
	}
	un, err := url.PathUnescape(u.URL.String())
	if err != nil {
		return nil, err
	}
	return json.Marshal(un)
}

func (u *sURL) UnmarshalJSON(data []byte) error {
	var s string
	var err error

	if err = json.Unmarshal(data, &s); err != nil {
		return err
	}

	if u.URL, err = url.Parse(s); err != nil {
		return err
	}

	return nil
}

// func (m *mount) loadDir() {
//
// 	m.Dir = m.URL.Path
// }

// exists checks if a directory exists.
func exists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return true
}
