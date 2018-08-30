package piplayer

// reqMessage defines structure of request messages for json api
type reqMessage struct {
	Component string            `json:"component"`
	Method    string            `json:"method"`
	Arguments map[string]string `json:"arguments"`
}

// resMessage defines structure for reponse messages for json api
type resMessage struct {
	Success bool        `json:"success"`
	Event   string      `json:"event"`
	Message interface{} `json:"message"`
}

// message defines the structure for a request and response messages for the websocket
type wsMessage struct {
	Component string            `json:"component"`
	Method    string            `json:"method"`
	Arguments map[string]string `json:"arguments"`
	Success   bool              `json:"success"`
	Event     string            `json:"event"`
	Message   interface{}       `json:"message"`
}
