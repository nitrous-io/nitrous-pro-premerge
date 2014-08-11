package events

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/nitrous-io/nitrous/controllers"
	"github.com/nitrous-io/nitrous/middleware"
	"github.com/nitrous-io/nitrous/pkg/httperror"
	"github.com/nitrous-io/nitrous/pkg/pubsub"
)

const (
	// Max time allowed to write a message to the client.
	writeWait = 5 * time.Second

	// Max time allowed to read the next pong message from client.
	pongWait = 30 * time.Second

	// Pings are sent to the client with this period.
	pingPeriod = (pongWait * 9) / 10
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	Error: func(w http.ResponseWriter, r *http.Request, status int, reason error) {
		// Do nothing. This is a workaround to prevent Upgrader from writing its
		// own plain/text HTTP response when Error is set to nil.
	},
}

func Stream(env *middleware.Env, c controllers.Helper, w http.ResponseWriter, r *http.Request) error {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		if e, ok := err.(websocket.HandshakeError); ok {
			return httperror.New(http.StatusBadRequest, "invalid_request", fmt.Sprintf("WebSocket handshake error: %v", e))
		}
		return err
	}

	go writer(ws, env.User.ID)
	reader(ws)

	return nil
}

func writer(ws *websocket.Conn, userID int64) {
	topic := pubsub.Topic(fmt.Sprintf("user-%d", userID))

	// FIXME This should come from the server/middleware.
	globalPS := pubsub.New(256)
	msgCh := globalPS.Subscribe(topic)
	globalPS.Publish("host created", topic)

	pingTicker := time.NewTicker(pingPeriod)
	defer func() {
		globalPS.Unsubscribe(msgCh, topic)
		pingTicker.Stop()
		ws.Close()
	}()

	for {
		select {
		case msg, ok := <-msgCh:
			if !ok {
				ws.SetWriteDeadline(time.Now().Add(writeWait))
				ws.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := ws.WriteJSON(msg); err != nil {
				return
			}
		case <-pingTicker.C:
			// Ping client periodically and expect pongs, else the connection
			// is closed.
			ws.SetWriteDeadline(time.Now().Add(writeWait))
			if err := ws.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}

func reader(ws *websocket.Conn) {
	defer ws.Close()
	ws.SetReadDeadline(time.Now().Add(pongWait))
	ws.SetPongHandler(func(string) error {
		ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	for {
		// We don't read anything except for pongs (handled by SetPongHandler).
		if _, _, err := ws.NextReader(); err != nil {
			break
		}
	}
}
