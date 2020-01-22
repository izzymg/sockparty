package sockparty_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/izzymg/sockparty"

	"golang.org/x/time/rate"
	"nhooyr.io/websocket"
)

var message = &struct {
	Event   string `json:"event"`
	Message string `json:"message"`
}{
	Event:   "bop",
	Message: "beep",
}

type client struct {
	Address string
	Conn    *websocket.Conn
}

func (c *client) connect() error {

	conn, _, err := websocket.Dial(context.TODO(), c.Address, nil)
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

func TestMany(t *testing.T) {
	var clients []*websocket.Conn

	party := sockparty.NewParty("many", &sockparty.Options{
		AllowedOrigin: "http://localhost:80",
		RateLimiter:   rate.NewLimiter(rate.Every(time.Millisecond*100), 3),
	})
	go party.Listen()

	stop := makeServer(party, "localhost:3500")
	defer func() {
		stop <- true
		party.Stop <- true
	}()

	for i := 0; i < 10; i++ {
		conn, _, err := websocket.Dial(context.Background(), "ws://localhost:3500", nil)
		if err != nil {
			t.Fatal(err)
		}
		clients = append(clients, conn)
	}

	<-time.After(time.Second * 3)

	// catch up
	connected := party.GetConnectedUserCount()
	if connected != len(clients) {
		t.Errorf("Expected %d connected, got %d", len(clients), connected)
	}

	for _, c := range clients {
		err := c.Close(websocket.StatusNormalClosure, "Bye")
		if err != nil {
			t.Fatal(err)
		}
	}

	<-time.After(time.Second * 3)

	connected = party.GetConnectedUserCount()
	if connected != 0 {
		t.Errorf("Expected 0 connected, got %d", connected)
	}
}

func TestContexts(t *testing.T) {
	party := sockparty.NewParty("Room room", &sockparty.Options{
		AllowedOrigin: "http://localhost:80",
	})

	go party.Listen()

	server := http.Server{
		Addr:         "localhost:3500",
		Handler:      party,
		ReadTimeout:  time.Second * 2,
		WriteTimeout: time.Second * 5,
		IdleTimeout:  time.Second * 2,
	}

	go func() {
		server.ListenAndServe()
	}()

	c := &client{
		Address: "ws://localhost:3500",
	}

	err := c.connect()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		mt, message, err := c.Conn.Read(ctx)
		if err == nil {
			t.Fatal("Expected cancel")
		}
		t.Logf("Message type %v: %v", mt, message)
	}()

	for {
		select {
		case <-time.After(time.Second * 2):
			cancel()
			<-time.After(time.Second * 2)
			party.Stop <- true
			server.Shutdown(context.TODO())
			return
		}
	}
}

func TestUpDown(t *testing.T) {

	party := sockparty.NewParty("Sick room dudes", &sockparty.Options{
		RateLimiter:   rate.NewLimiter(rate.Every(time.Millisecond*100), 5),
		AllowedOrigin: "http://localhost:80",
	})
	go party.Listen()

	// TODO: get random free port
	stop := makeServer(party, "localhost:3500")
	c := &client{
		Address: "ws://localhost:3500",
	}

	defer func() {
		stop <- true
	}()

	err := c.connect()
	if err != nil {
		t.Fatal(err)
	}

	one := time.After(time.Second * 1)
	two := time.After(time.Second * 2)

	select {
	case <-one:
		c.disconnect(websocket.StatusNormalClosure, "Bye")
	case <-two:
		count := party.GetConnectedUserCount()
		if count != 0 {
			t.Fatalf("Expected 0 users, got %d", count)
		}
		return
	}
}
