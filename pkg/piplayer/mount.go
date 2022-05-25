package piplayer

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"runtime"
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

func (m *mount) loadDir() {
	if runtime.GOOS != "linux" {
		m.Dir = m.URL.Path
		return
	}
	m.Dir = fmt.Sprintf(gvfsStr, sharesDir, m.URL.Host, m.URL.Path[1:])
}

// mounted checks if the mountpoint defined is mounted.
func (m *mount) mounted() bool {
	var share string
	i := strings.Index(m.URL.Path[1:], "/")
	if i == -1 {
		share = m.URL.Path
	} else {
		share = m.URL.Path[1 : i+1]
	}

	mnt := fmt.Sprintf(gvfsStr, sharesDir, m.URL.Host, share)
	return exists(mnt)
}

// exists checks if a directory exists.
func exists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	return true
}

// unmount a gvfs drive using the `gio`command.
func (m *mount) unmount() error {
	if runtime.GOOS != "linux" {
		return errors.New("can't unmount on non-linux environment in this build")
	}
	cmd := exec.Command("gio", "mount", "-u", m.URL.String())
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// mount a network drive in gvfs using the `gio` command.
func (m *mount) mount() error {
	if m.mounted() {
		return nil
	}

	if runtime.GOOS != "linux" {
		return errors.New("can't mount on non-linux environment in this build")
	}

	cmd := exec.Command("gio", "mount", m.URL.String())
	ip, err := cmd.StdinPipe()
	if err != nil {
		log.Println("mount(): error trying to get StdinPipe for mount command")
		return err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		log.Println("mount(): error trying to start mount command")
		return err
	}

	auth := m.Username + "\n" + m.Domain + "\n" + m.Password + "\n"
	if _, err := ip.Write([]byte(auth)); err != nil {
		log.Println("error authenticating mount")
		return err
	}

	cmd.Wait()

	return nil
}
