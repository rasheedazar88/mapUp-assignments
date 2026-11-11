// Package realtime broadcasts alert messages to WebSocket clients.
//
// How the concurrency works:
//
//   - ONE goroutine (Hub.Run) owns the set of connected clients. Nothing else
//     reads or writes that map, so it needs no mutex and cannot race.
//   - Other goroutines talk to the hub only through channels:
//     register (a client connected), unregister (a client left), broadcast (a new alert).
//   - Each client has its own buffered `send` channel and a writer goroutine.
//     The hub never writes to a network socket itself; it drops the message
//     into each client's channel. A slow or dead client therefore cannot
//     block the hub or the other clients.
//
// A goroutine is a very cheap thread managed by the Go runtime. A channel is a
// typed, thread-safe queue that goroutines use to hand values to each other.
package realtime

import (
	"context"
	"log/slog"
	"sync/atomic"
)

type Hub struct {
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
	done       chan struct{}        // closed when Run exits, so senders never block forever
	clients    map[*Client]struct{} // owned by Run(); never touched elsewhere
	count      atomic.Int64         // safe to read from any goroutine (metrics/tests)
}

func NewHub() *Hub {
	return &Hub{
		register:   make(chan *Client),
		unregister: make(chan *Client),
		// Buffered: Publish can queue up to 256 messages without waiting for Run.
		broadcast: make(chan []byte, 256),
		done:      make(chan struct{}),
		clients:   make(map[*Client]struct{}),
	}
}

// join/leave hand a client to the hub, but give up if the hub has stopped.
// Without the `done` case, a goroutine would block forever after shutdown.
func (h *Hub) join(c *Client) bool {
	select {
	case h.register <- c:
		return true
	case <-h.done:
		return false
	}
}

func (h *Hub) leave(c *Client) {
	select {
	case h.unregister <- c:
	case <-h.done:
	}
}

// Run is the hub's event loop. Start it once with `go hub.Run(ctx)`.
// `select` waits on several channels at once and handles whichever is ready first.
func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case c := <-h.register:
			h.clients[c] = struct{}{}
			h.count.Store(int64(len(h.clients)))
			slog.Info("ws client connected", "clients", len(h.clients), "remote", c.remote)

		case c := <-h.unregister:
			h.remove(c)

		case msg := <-h.broadcast:
			for c := range h.clients {
				select {
				case c.send <- msg: // queued for this client's writer goroutine
				default:
					// Client's buffer is full: it is not reading. Disconnect it
					// rather than letting one slow client delay everybody.
					slog.Warn("ws client too slow, disconnecting", "remote", c.remote)
					h.remove(c)
				}
			}

		case <-ctx.Done():
			// Server shutting down: closing each send channel makes its writer
			// goroutine send a close frame and exit.
			for c := range h.clients {
				h.remove(c)
			}
			close(h.done)
			return
		}
	}
}

func (h *Hub) remove(c *Client) {
	if _, ok := h.clients[c]; !ok {
		return // already removed (e.g. both "too slow" and "disconnected" fired)
	}
	delete(h.clients, c)
	close(c.send)
	h.count.Store(int64(len(h.clients)))
	slog.Info("ws client disconnected", "clients", len(h.clients), "remote", c.remote)
}

// Publish queues a message for broadcast and returns immediately, so the HTTP
// handler that calls it (POST /vehicles/location) is never blocked by WebSocket
// delivery. If the queue is full (extreme load), the message is dropped from
// the live feed. It is already saved in the database, so clients still see it
// in history and in GET /alerts/events.
func (h *Hub) Publish(msg []byte) {
	select {
	case h.broadcast <- msg:
	default:
		slog.Warn("alert broadcast queue full; message dropped from live feed (still persisted)")
	}
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int { return int(h.count.Load()) }
