package piPlayer

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

const (
	readBufferSize  = 1024
	writeBufferSize = 1024
)

var upgrader = &websocket.Upgrader{
	ReadBufferSize:  readBufferSize,
	WriteBufferSize: writeBufferSize,
}

// connectionWS has the websocket connection to the control page
type connectionWS struct {
	conn   *websocket.Conn
	send   chan resMessage
	active bool
}

// HandleWebsocket handles websocket connections
func (c *connectionWS) HandlerWebsocket(w http.ResponseWriter, r *http.Request) {
	var err error
	c.conn, err = upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error trying to upgrade to websocket connection:", err)
		c.active = false
		return
	}

	c.active = true
	go c.write()
	// we can't start the read() method on a separate goroutine, or this function would return and stop serving the websocket connections
	// we need the infinite loop in the read function to block operations and keep this function alive
	c.read()
}

func (c *connectionWS) write() {
	defer c.conn.Close()

	// this loop keeps running as long as the channel is open.
	for msg := range c.send {
		err := c.conn.WriteJSON(msg)
		if err != nil {
			log.Println("Error trying to write JSON to the socket: ", err)
			// this probably means that the connection is broken,
			// so close the channel and break out of the loop.
			close(c.send)
			c.active = false
			break
		}
	}
}

// read reads the messages from the socket
func (c *connectionWS) read() {
	defer c.conn.Close()

	for {
		var msg reqMessage
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			log.Println("Error trying to read the JSON from the socket: ", err)
			// this probably means that the connection is broken,
			// so close the channel and break out of the loop.
			close(c.send)
			c.active = false
			break
		}

		// ignore socket messages for now.
		// TODO: handle socket messages.
		log.Println("socket message received: ", msg)
	}
}
