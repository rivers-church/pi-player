package piPlayer

import (
	"encoding/json"
	"io/ioutil"
)

// Config holds the configuration of the pi-player
type Config struct {
	Directory   string
	Location    string
	AudioOutput string
	Remote      remote
}

// Load reads the config file and unmarshalls it to the config struct
func (conf *Config) Load(path string) error {
	if path == "" {
		path = "config.json"
	}

	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, &conf)
}

// Save reads the config struct, marshalls it and writes it to the config file
func (conf *Config) Save(path string) error {
	if path == "" {
		path = "config.json"
	}

	jconf, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(path, jconf, 0600)
}
