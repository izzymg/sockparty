package sockparty_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/izzymg/sockparty"
	"nhooyr.io/websocket"
)

func spawn(messageCount int, got chan int) func() {

	// Create a connection
	conn, _, err := websocket.Dial(context.Background(), "ws://localhost:3500", nil)
	if err != nil {
		panic(err)
	}

	// Write n messages to it
	go func() {
		for i := 0; i < messageCount; i++ {
			<-time.After(time.Millisecond * 10)
			err := conn.Write(context.Background(), websocket.MessageText, []byte(`{ "event": "message", "data": "bah" }`))
			if err != nil {
				panic(err)
			}
		}
	}()

	// Read messages and send them back
	go func() {
		for {
			_, _, err := conn.Read(context.TODO())
			if err != nil {
				// TODO: find better handling for this case
				return
			}
			got <- 1
		}
	}()

	return func() {
		conn.Close(websocket.StatusNormalClosure, "Going away")
	}
}

// Bully an echo server
func TestBully(t *testing.T) {

	connectionCount := 10
	messagesPerConnection := 200

	// Create party, can't ping as client doesn't implement pong
	party := sockparty.NewParty("", &sockparty.Options{
		PingFrequency: 0,
		RateLimiter:   rate.NewLimiter(rate.Every(time.Millisecond*10), 10),
	})

	party.SetMessageEvent("message", func(party *sockparty.Party, message sockparty.IncomingMessage) {
		party.SendMessage <- sockparty.OutgoingMessage{UserID: message.UserID, Payload: "echo"}
	})

	party.UserInvalidMessageHandler = func(message sockparty.IncomingMessage) {
		panic(fmt.Errorf("Test: Invalid message"))
	}

	go party.Listen()
	defer func() {
		party.StopListening <- true
	}()

	// Create HTTP server
	server := http.Server{
		Addr:    "localhost:3500",
		Handler: party,
	}
	defer server.Shutdown(context.Background())

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			panic(err)
		}
	}()

	// Setup connection vars
	got := make(chan int, connectionCount)
	messagesReceived := 0

	for i := 0; i < connectionCount; i++ {
		cleanup := spawn(messagesPerConnection, got)
		defer cleanup()
	}

	// Start incrementing connections received
	going := true
	for going {
		select {
		case <-got:
			messagesReceived++
			if messagesReceived == connectionCount*messagesPerConnection {
				going = false
			}
		}
	}
	// Assert that got as many messages as connections were echoed back
	if messagesReceived != connectionCount*messagesPerConnection {
		log.Fatalf("Expected to recieve %d messages but got %d", connectionCount*messagesPerConnection, messagesReceived)
	}
}

func BenchmarkBully(b *testing.B) {

	connectionCount := 10
	messagesPerConnection := 200

	// Create party, can't ping as client doesn't implement pong
	party := sockparty.NewParty("", &sockparty.Options{
		PingFrequency: 0,
		RateLimiter:   rate.NewLimiter(rate.Every(time.Millisecond), 1),
	})

	party.SetMessageEvent("message", func(party *sockparty.Party, message sockparty.IncomingMessage) {
		party.SendMessage <- sockparty.OutgoingMessage{Broadcast: true, Payload: "bcast"}
	})

	party.UserInvalidMessageHandler = func(message sockparty.IncomingMessage) {
		panic(fmt.Errorf("Test: Invalid message"))
	}

	go party.Listen()
	defer func() {
		party.StopListening <- true
	}()

	// Create HTTP server
	server := http.Server{
		Addr:    "localhost:3500",
		Handler: party,
	}
	defer server.Shutdown(context.Background())

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			panic(err)
		}
	}()

	got := make(chan int)
	// Create conns
	for i := 0; i < connectionCount; i++ {
		cleanup := spawn(messagesPerConnection, got)
		defer cleanup()
	}
}
