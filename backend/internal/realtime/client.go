package realtime

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second    // max time to write one message
	pongWait   = 60 * time.Second    // client must answer a ping within this time
	pingPeriod = (pongWait * 9) / 10 // send pings a bit more often than pongWait
	maxInbound = 1024                // clients don't send data; cap anything they do send
	sendBuffer = 64                  // messages queued per client before it counts as "too slow"
)

// Client is one WebSocket connection. It runs two goroutines:
//
//	readPump  — reads from the socket. Its only jobs are to notice disconnects
//	            and to process pong replies (the heartbeat).
//	writePump — the ONLY goroutine that writes to the socket. Gorilla WebSocket
//	            allows at most one concurrent writer, so funnelling all writes
//	            (alerts + pings) through one goroutine keeps it safe.
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	remote string
}

// Handler upgrades GET /ws/alerts to a WebSocket and registers the client.
// allowOrigin decides which browser origins may connect (same list as CORS).
func Handler(hub *Hub, allowOrigin func(origin string) bool) http.HandlerFunc {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			return origin == "" || allowOrigin(origin) // no Origin = non-browser client (curl, wscat, tests)
		},
	}
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			slog.Warn("ws upgrade failed", "err", err)
			return // Upgrade already wrote an HTTP error response
		}
		c := &Client{hub: hub, conn: conn, send: make(chan []byte, sendBuffer), remote: r.RemoteAddr}
		if !hub.join(c) {
			conn.Close() // server is shutting down
			return
		}
		go c.writePump()
		go c.readPump()
	}
}

func (c *Client) readPump() {
	defer func() {
		c.hub.leave(c) // tells the hub to forget us; the hub closes c.send
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxInbound)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		// Each pong proves the client is alive; extend the deadline.
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		// We ignore client messages; ReadMessage returns an error when the
		// client disconnects or misses the pong deadline, which ends the loop.
		if _, _, err := c.conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed our channel (we were removed, or the server is shutting down).
				_ = c.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseGoingAway, ""))
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
