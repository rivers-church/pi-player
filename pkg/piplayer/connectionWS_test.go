package piplayer

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// wsTestServer starts a server whose only route is the websocket handler for c.
func wsTestServer(t *testing.T, c ConnectionWS) (*httptest.Server, string) {
	t.Helper()
	p := &Player{conf: &Config{}}
	server := httptest.NewServer(c.HandlerWebsocket(p))
	t.Cleanup(server.Close)
	return server, "ws" + strings.TrimPrefix(server.URL, "http")
}

func dial(t *testing.T, url string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dialing %s failed: %v", url, err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// TestTrySendWithNoConnectionDoesNotBlock is the regression test for handlers
// parking forever on an unbuffered channel with no websocket reader. Against
// the old code this hangs until the test binary times out.
func TestTrySendWithNoConnectionDoesNotBlock(t *testing.T) {
	c := NewConnWS()

	done := make(chan bool, 1)
	go func() {
		done <- c.trySend(wsMessage{Component: "playlist", Event: "newItems"})
	}()

	select {
	case delivered := <-done:
		if delivered {
			t.Error("trySend reported delivery with no websocket connected")
		}
	case <-time.After(time.Second):
		t.Fatal("trySend blocked with no websocket connected")
	}
}

func TestTrySendDeliversToConnectedBrowser(t *testing.T) {
	c := NewConnWS()
	_, url := wsTestServer(t, c)
	conn := dial(t, url)

	waitActive(t, c)
	if !c.trySend(wsMessage{Component: "playlist", Event: "newItems"}) {
		t.Fatal("trySend did not deliver to a connected browser")
	}

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var got wsMessage
	if err := conn.ReadJSON(&got); err != nil {
		t.Fatalf("reading the pushed message failed: %v", err)
	}
	if got.Event != "newItems" {
		t.Errorf("got event %q, want %q", got.Event, "newItems")
	}
}

// TestTakeoverClosesOldConnection covers the takeover path: the first browser
// must get the farewell message and a close frame, and messages sent afterwards
// must go to the second browser only.
func TestTakeoverClosesOldConnection(t *testing.T) {
	c := NewConnWS()
	_, url := wsTestServer(t, c)

	first := dial(t, url)
	waitActive(t, c)
	second := dial(t, url)
	waitActive(t, c)

	first.SetReadDeadline(time.Now().Add(2 * time.Second))
	var farewell wsMessage
	if err := first.ReadJSON(&farewell); err != nil {
		t.Fatalf("the replaced browser did not get a farewell: %v", err)
	}
	if farewell.Event != "disconnect" {
		t.Errorf("farewell event is %q, want %q", farewell.Event, "disconnect")
	}
	if _, _, err := first.ReadMessage(); err == nil {
		t.Error("the replaced connection is still open, want it closed")
	}

	if !c.trySend(wsMessage{Component: "playlist", Event: "newItems"}) {
		t.Fatal("trySend did not deliver after the takeover")
	}
	second.SetReadDeadline(time.Now().Add(2 * time.Second))
	var got wsMessage
	if err := second.ReadJSON(&got); err != nil {
		t.Fatalf("the new browser did not receive the message: %v", err)
	}
	if got.Event != "newItems" {
		t.Errorf("got event %q, want %q", got.Event, "newItems")
	}
}

// TestRepeatedTakeoversAreRaceFree exists for the race detector: overlapping
// connects and pushes used to write to the same socket from two goroutines,
// which makes gorilla panic.
func TestRepeatedTakeoversAreRaceFree(t *testing.T) {
	c := NewConnWS()
	_, url := wsTestServer(t, c)

	stop := make(chan struct{})
	pushed := make(chan struct{})
	go func() {
		defer close(pushed)
		for {
			select {
			case <-stop:
				return
			default:
				c.trySend(wsMessage{Component: "playlist", Event: "newItems"})
			}
		}
	}()

	for range 10 {
		conn := dial(t, url)
		conn.Close()
	}
	close(stop)
	<-pushed
}

func TestHandlerRejectsNonWebsocketRequest(t *testing.T) {
	c := NewConnWS()
	p := &Player{conf: &Config{}}
	recorder := httptest.NewRecorder()

	c.HandlerWebsocket(p).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/ws/viewer", nil))

	if recorder.Code == http.StatusOK {
		t.Error("a plain GET was accepted as a websocket upgrade")
	}
	if c.isActive() {
		t.Error("a failed upgrade left the connection marked active")
	}
}

// waitActive waits for the handler goroutine to finish registering the
// connection, which happens just after the upgrade completes.
func waitActive(t *testing.T, c ConnectionWS) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c.isActive() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("websocket connection never became active")
}
