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
		// An empty byte slice isn't valid JSON: it makes the whole config
		// fail to marshal with "unexpected end of JSON input".
		return []byte("null"), nil
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

// exists checks if a directory exists. Anything that isn't a plain "not
// there" - a permission problem, a broken network mount - counts as missing
// too, because the player can't read it either way.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
