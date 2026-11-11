package realtime

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Unit test for the hub alone (no database): broadcast fan-out to many
// concurrent clients, disconnect cleanup, and shutdown. Run with -race.
func TestHubBroadcastAndLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := NewHub()
	go hub.Run(ctx)

	srv := httptest.NewServer(Handler(hub, func(string) bool { return true }))
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http")

	const n = 20
	conns := make([]*websocket.Conn, n)
	for i := range conns {
		c, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			t.Fatal(err)
		}
		conns[i] = c
	}
	waitFor(t, func() bool { return hub.ClientCount() == n })

	hub.Publish([]byte(`{"event_id":"evt_1"}`))

	var wg sync.WaitGroup
	for _, c := range conns {
		wg.Add(1)
		go func(c *websocket.Conn) {
			defer wg.Done()
			_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
			_, msg, err := c.ReadMessage()
			if err != nil || string(msg) != `{"event_id":"evt_1"}` {
				t.Errorf("client got %q, %v", msg, err)
			}
		}(c)
	}
	wg.Wait()

	// Half the clients leave; the hub notices and forgets them.
	for _, c := range conns[:n/2] {
		c.Close()
	}
	waitFor(t, func() bool { return hub.ClientCount() == n/2 })

	// Shutdown closes the remaining connections.
	cancel()
	waitFor(t, func() bool { return hub.ClientCount() == 0 })
	for _, c := range conns[n/2:] {
		_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
		if _, _, err := c.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseGoingAway) {
			t.Errorf("expected close frame on shutdown, got %v", err)
		}
	}
}

func TestPublishNeverBlocks(t *testing.T) {
	hub := NewHub() // Run is NOT started: nobody drains the queue
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ { // far more than the 256-slot buffer
			hub.Publish([]byte("x"))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked")
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
