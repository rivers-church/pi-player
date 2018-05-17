package piplayer

type remote struct {
	Name    string
	Vendor  uint16
	Product uint16
}

var remoteCommands = map[string]string{
	"KEY_HOME":         "",
	"KEY_INFO":         "",
	"KEY_UP":           "",
	"KEY_DOWN":         "",
	"KEY_LEFT":         "",
	"KEY_RIGHT":        "",
	"KEY_ENTER":        "",
	"KEY_BACK":         "",
	"KEY_CONTEXT_MENU": "",
	"KEY_PLAYPAUSE":    "pauseResume",
	"KEY_STOP":         "quit",
	"KEY_REWIND":       "seekBack30",
	"KEY_FASTFORWARD":  "seekForward30",
}
