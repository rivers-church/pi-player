package piplayer

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait       = 10 * time.Second
	pongWait        = 60 * time.Second
	pingPeriod      = (pongWait * 9) / 10
	maxMessageSize  = 512
	readBufferSize  = 1024
	writeBufferSize = 1024
)

var upgrader = &websocket.Upgrader{
	ReadBufferSize:  readBufferSize,
	WriteBufferSize: writeBufferSize,
}

// ConnectionWS represents a WebSocket connection.
type ConnectionWS interface {
	HandlerWebsocket(*Player) http.HandlerFunc
	read()
	write()
	getChanSend() chan wsMessage
	getChanReceive() chan wsMessage
	isActive() bool
}

// connWS represents a WebSocket connection.
type connWS struct {
	conn    *websocket.Conn
	send    chan wsMessage
	receive chan wsMessage
	active  bool
}

// NewConnection returns a connection with open send and receive channels
// func NewConnection() ConnectionWS {
// 	conn := ConnectionWS{
// 		send:    make(chan resMessage),
// 		receive: make(chan reqMessage),
// 	}

// 	return conn
// }

// NewConnWS returns a new websocket connection struct.
func NewConnWS() ConnectionWS {
	return &connWS{}
}

func (c *connWS) getChanSend() chan wsMessage {
	return c.send
}

func (c *connWS) getChanReceive() chan wsMessage {
	return c.receive
}

func (c *connWS) isActive() bool {
	return c.active
}

// HandlerWebsocket handles websocket connections for the browser viewer and controller.
func (c *connWS) HandlerWebsocket(p *Player) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If connection is already active, then close it gracefully, and create a new one.
		if c.active {
			msg := wsMessage{
				Event:   "disconnect",
				Success: true,
				Message: "Another device has taken over the connection. Login again to take it back.",
			}

			if err := c.conn.WriteJSON(msg); err != nil {
				log.Printf("Error writting disconnect message: ConnectionWS.HandlerWebsocket: %v\n", err)
			}

			if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
				log.Printf("Error writting close message: ConnectionWS.HandlerWebsocket(): %v\n", err)
			}

			c.conn.Close()
		}

		var err error
		c.conn, err = upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("Error trying to upgrade to websocket connection:", err)
			c.active = false
			return
		}

		log.Println("Websocket connection being handled for ", r.URL.Path)

		c.send = make(chan wsMessage)
		c.receive = make(chan wsMessage)

		c.active = true
		go c.write()
		go c.read()
		go p.HandleWebSocketMessage()
	}
}

// write sends data to the websocket.
func (c *connWS) write() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
		c.active = false
	}()

	// This loop keeps running as long as the channel is open.
	for {
		select {
		// Send a message from the send channel to the websocket.
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					log.Printf("Error writting close message: ConnectionWS.write(): %v\n", err)
				}
				return
			}
			err := c.conn.WriteJSON(msg)
			if err != nil {
				log.Println("Error trying to write JSON to the socket: ", err)
				// this probably means that the connection is broken,
				// so close the channel and break out of the loop.
				close(c.send)
				c.active = false
				return
			}
		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Println("Error trying to send ping message")
				// not sure what else needs to be done here.
				return
			}
		}
	}
}

// read reads the messages from the socket.
func (c *connWS) read() {
	defer c.conn.Close()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		var msg wsMessage
		err := c.conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Println("Error, websocket closed: ", err)
			} else {
				log.Println("Error trying to read the JSON from the socket: ", err)
			}
			close(c.receive)
			c.active = false
			break
		}

		// Handle socket messages.
		log.Println("socket message received: ", msg)
		c.receive <- msg
	}
}
