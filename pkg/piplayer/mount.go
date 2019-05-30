package piplayer

import (
	"log"
	"os/exec"
)

func mount(location, username, domain, password string) error {
	cmd := exec.Command("gio", "mount", location)
	p, err := cmd.StdinPipe()
	if err != nil {
		log.Println("error tring to get SdtinPipe for mount command")
		return err
	}
	if err := cmd.Start(); err != nil {
		log.Println("error trying to start mount command")
		return err
	}

	auth := username + "\n" + domain + "\n" + password + "\n"
	if _, err := p.Write([]byte(auth)); err != nil {
		log.Println("error authenticating mount")
		return err
	}

	cmd.Wait()

	return nil
}
