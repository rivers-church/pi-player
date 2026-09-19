package piplayer

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait       = 10 * time.Second
	pongWait        = 60 * time.Second
	pingPeriod      = (pongWait * 9) / 10
	readBufferSize  = 1024
	writeBufferSize = 1024
	// sendBuffer gives producers a little room so a brief stall in the writer
	// doesn't cost messages. Anything beyond this is dropped rather than
	// blocking the caller.
	sendBuffer = 16
	// closeWait is how long a takeover waits for the previous writer to say
	// goodbye and let go of the socket.
	closeWait = writeWait + time.Second
)

var upgrader = &websocket.Upgrader{
	ReadBufferSize:  readBufferSize,
	WriteBufferSize: writeBufferSize,
}

// ConnectionWS represents a WebSocket connection to one browser page.
//
// NOTE: this is an outbound-only path. The server pushes messages to the
// browser and never consumes structured messages back over the socket; the
// browser talks back over the JSON API instead.
type ConnectionWS interface {
	HandlerWebsocket(p *Player) http.HandlerFunc
	trySend(msg wsMessage) bool
	isActive() bool
	closeCurrent(farewell wsMessage)
}

// wsWriter is one browser connection and the goroutine that writes to it.
// The goroutine owns conn: nothing else may write to the socket, because
// gorilla/websocket panics on concurrent writes.
type wsWriter struct {
	conn     *websocket.Conn
	stop     chan wsMessage // buffered, holds a parting message; closed to stop the writer
	finished chan struct{}  // closed when the writer goroutine has returned
}

// connWS represents a WebSocket connection.
type connWS struct {
	mu   sync.Mutex
	cur  *wsWriter // nil when no browser is connected
	send chan wsMessage
}

// NewConnWS returns a new websocket connection struct.
func NewConnWS() ConnectionWS {
	// NOTE: interfaces are always pointers...
	// So I have to return a pointer here.
	return &connWS{
		send: make(chan wsMessage, sendBuffer),
	}
}

func (c *connWS) isActive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cur != nil
}

// trySend hands msg to the websocket writer. It never blocks: if no browser is
// connected, or the writer has fallen behind, the message is dropped and false
// is returned. Dropping is the right call here - every message is a nudge to
// refresh or to show item N, and the page re-syncs with getItems when it
// reconnects.
func (c *connWS) trySend(msg wsMessage) bool {
	if !c.isActive() {
		return false
	}

	select {
	case c.send <- msg:
		return true
	default:
		log.Printf("websocket send buffer is full, dropping message: %s/%s\n", msg.Component, msg.Event)
		return false
	}
}

// closeCurrent sends farewell to the connected browser, if there is one, and
// waits for the writer to release the socket.
func (c *connWS) closeCurrent(farewell wsMessage) {
	c.mu.Lock()
	w := c.cur
	c.cur = nil
	c.mu.Unlock()

	if w == nil {
		return
	}

	// stop is buffered and swapped out under the mutex, so exactly one
	// goroutine ever sends on it and this cannot block.
	w.stop <- farewell
	close(w.stop)

	select {
	case <-w.finished:
	case <-time.After(closeWait):
		log.Println("timed out waiting for the previous websocket writer to finish")
	}
}

// HandlerWebsocket handles websocket connections for the browser viewer and controller.
func (c *connWS) HandlerWebsocket(p *Player) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// If a connection is already active, close it gracefully before
		// taking over. The previous writer sends the farewell itself; writing
		// to the socket from here would race it.
		if c.isActive() {
			if p.conf.DebugEnabled() {
				log.Printf("new websocket connection request while previous request was active. Closing current connection.")
			}

			c.closeCurrent(wsMessage{
				Component: "connection",
				Event:     "disconnect",
				Success:   true,
				Message:   "Another device has taken over the connection. Login again to take it back.",
			})
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("Error trying to upgrade to websocket connection:", err)
			return
		}

		log.Println("Websocket connection being handled for ", r.URL.Path)

		// Messages queued for the connection that just went away are stale.
		c.drain()

		writer := &wsWriter{
			conn:     conn,
			stop:     make(chan wsMessage, 1),
			finished: make(chan struct{}),
		}

		c.mu.Lock()
		c.cur = writer
		c.mu.Unlock()

		go c.write(writer)
	}
}

// drain empties the send buffer without blocking.
func (c *connWS) drain() {
	for {
		select {
		case <-c.send:
		default:
			return
		}
	}
}

// retire clears the connection state, but only if w is still the live writer.
// Without the check a writer that is shutting down could wipe out the
// connection that replaced it.
func (c *connWS) retire(w *wsWriter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cur == w {
		c.cur = nil
	}
}

// write sends data to the websocket. It owns w.conn for the life of the
// goroutine and is the only writer to it.
func (c *connWS) write(w *wsWriter) {
	log.Printf("Starting write() goroutine\n")

	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		w.conn.Close()
		c.retire(w)
		close(w.finished)
	}()

	// This loop keeps running as long as the connection lives.
	for {
		select {
		// Send a message from the send channel to the websocket.
		case msg := <-c.send:
			w.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := w.conn.WriteJSON(msg); err != nil {
				log.Printf("error trying to write JSON to the socket: %v\n", err)
				// this probably means that the connection is broken,
				return
			}
		case <-ticker.C:
			w.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := w.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Printf("error trying to send ping message. Exiting goroutine: %v\n", err)
				return
			}
		case farewell, ok := <-w.stop:
			if ok {
				w.conn.SetWriteDeadline(time.Now().Add(writeWait))
				if err := w.conn.WriteJSON(farewell); err != nil {
					log.Printf("error writing disconnect message: ConnectionWS.write: %v\n", err)
				}
			}
			w.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := w.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
				log.Printf("error writing close message: ConnectionWS.write: %v\n", err)
			}
			return
		}
	}
}
