package piplayer

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"strings"
)

const sharesDir = "/run/user/1000/gvfs/"
const gvfsStr = "%ssmb-share:server=%s,share=%s"

// sURL is copy of url.URL but with its own JSON marshalling and unmarshalling methods.
type sURL struct {
	*url.URL
}

// Mount holds the details of the network or usb mount location.
type mount struct {
	Dir      string `json:"-"`
	URL      sURL
	Username string
	Domain   string
	Password string
}

func (u sURL) MarshalJSON() ([]byte, error) {
	return json.Marshal(u.URL.String())
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

func (m *mount) mounted() bool {
	var share string
	i := strings.Index(m.URL.Path[1:], "/")
	if i == -1 {
		share = m.URL.Path
	} else {
		share = m.URL.Path[1 : i+1]
	}

	mnt := fmt.Sprintf(gvfsStr, sharesDir, m.URL.Host, share)
	m.Dir = fmt.Sprintf(gvfsStr, sharesDir, m.URL.Host, m.URL.Path[1:])
	return exists(mnt)
}

func exists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return true
}

func (m *mount) unmount() error {
	cmd := exec.Command("gio", "mount", "-u", m.URL.String())
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

func (m *mount) mount() error {
	if m.mounted() {
		return nil
	}

	if err := m.unmount(); err != nil {
		log.Printf("Error trying to unmount share (%s):\n%v\n", m.URL, err)
	}

	cmd := exec.Command("gio", "mount", m.URL.String())
	p, err := cmd.StdinPipe()
	if err != nil {
		log.Println("error tring to get SdtinPipe for mount command")
		return err
	}
	if err := cmd.Start(); err != nil {
		log.Println("error trying to start mount command")
		return err
	}

	auth := m.Username + "\n" + m.Domain + "\n" + m.Password + "\n"
	if _, err := p.Write([]byte(auth)); err != nil {
		log.Println("error authenticating mount")
		return err
	}

	cmd.Wait()

	// TODO: unmount previous mount

	return nil
}
