package sockroom_test

import (
	"context"
	"fmt"
	"github.com/izzymg/sockroom"
	"net/http"
	"testing"
	"time"

	"golang.org/x/time/rate"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type client struct {
	Address string
	Conn    *websocket.Conn
}

func (c *client) connect() error {

	// Create a client to connect to the room with
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, c.Address, nil)
	if err != nil {
		return fmt.Errorf("Websocket dial failed: %v", err)
	}
	c.Conn = conn
	return nil
}

func (c *client) disconnect(code websocket.StatusCode, reason string) error {
	if c.Conn == nil {
		return fmt.Errorf("Disconnect called on nil client")
	}
	err := c.Conn.Close(code, reason)
	if err != nil {
		return fmt.Errorf("Disconnect failed: %w", err)
	}
	return nil
}

// Non blocking http server helper
func makeServer(handler http.Handler, address string) chan<- bool {

	server := &http.Server{
		Handler: handler,
		Addr:    address,
	}

	// Start the server
	stopServer := make(chan bool)
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			panic(err)
		}
	}()
	go func() {
		<-stopServer
		server.Shutdown(context.TODO())
	}()
	return stopServer
}

func TestConnect(t *testing.T) {

	message := &struct {
		Event   string `json:"event"`
		Message string `json:"message"`
	}{
		Event:   "bop",
		Message: "beep",
	}

	party := sockroom.NewParty("Sick room dudes", &sockroom.Options{
		RateLimiter:   rate.NewLimiter(rate.Every(time.Millisecond*100), 5),
		AllowedOrigin: "http://localhost:80",
	})

	// TODO: get random free port
	stop := makeServer(party, "localhost:3500")
	client := &client{
		Address: "ws://localhost:3500",
	}

	err := client.connect()
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		t.Log("Stopping clean")
		stop <- true
	}()

	five := time.After(time.Second * 5)
	six := time.After(time.Second * 6)

	select {
	case <-five:
		client.disconnect(websocket.StatusNormalClosure, "Bye")
	case <-six:
		count := party.GetConnectedUserCount()
		if count != 0 {
			t.Fatalf("Expected 0 users, got %d", count)
		}
		return
	}

	client.connect()

	// spam
	ticker := time.NewTicker(time.Millisecond * 10)
	timeout := time.NewTimer(time.Second * 2)
	for {
		select {
		case <-ticker.C:
			wsjson.Write(context.Background(), client.Conn, message)
		case <-timeout.C:
			client.disconnect(websocket.StatusNormalClosure, "Cya")
			return
		}
	}

}
