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
