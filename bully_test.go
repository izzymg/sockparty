package sockparty_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"github.com/izzymg/sockparty"
	"nhooyr.io/websocket"
)

func getAddress() string {
	if addr, ok := os.LookupEnv("TEST_ADDR"); ok {
		return addr
	}
	return "localhost:3500"
}

// Simple HTTP server to serve parties
func basicPartyServer(party *sockparty.Party, stop chan bool) {
	server := http.Server{
		Addr:    getAddress(),
		Handler: party,
	}

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			panic(err)
		}
	}()

	select {
	case <-stop:
		server.Shutdown(context.Background())
	}
}

/* Spawn a websocket connection and send n messages, writing back to 'got'
each time a message is receieved */
func spawnrw(messageCount int, got chan int) func() {
	addr := fmt.Sprintf("ws://%s", getAddress())
	// Create a connection
	conn, _, err := websocket.Dial(context.Background(), addr, nil)
	if err != nil {
		panic(err)
	}

	// Write n messages to it
	go func() {
		for i := 0; i < messageCount; i++ {
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

	// Return cleanup function
	return func() {
		conn.Close(websocket.StatusNormalClosure, "Going away")
	}
}

// Spawn n conns
func bully(connCount int, messagesPerConn int) int {
	got := make(chan int)
	messagesReceived := 0

	// Spawn n websocket connection loops
	for i := 0; i < connCount; i++ {
		cleanup := spawnrw(messagesPerConn, got)
		defer cleanup()
	}

	// Start incrementing connections received
	for {
		select {
		case <-got:
			messagesReceived++
			if messagesReceived == connCount*messagesPerConn {
				return messagesReceived
			}
		}
	}
}

// Bully an active server
func BenchmarkRealBully(b *testing.B) {
	connCount := 3
	messagesPerConn := 100

	got := make(chan int)
	// Spawn n websocket connection loops
	for i := 0; i < connCount; i++ {
		cleanup := spawnrw(messagesPerConn, got)
		defer cleanup()
	}

	// Start incrementing connections received
	for {
		select {
		case <-got:
			continue
		case <-time.After(time.Second * 30):
			return
		}
	}
}

// Bully an echo server
func TestEchoBully(t *testing.T) {

	// Create party, can't ping as client doesn't implement pong
	party := sockparty.NewParty("", &sockparty.Options{
		PingFrequency: 0,
		RateLimiter:   rate.NewLimiter(rate.Every(time.Millisecond*10), 10),
	})

	party.SetMessageEvent("message", func(party *sockparty.Party, message sockparty.IncomingMessage) {
		party.SendMessage <- sockparty.OutgoingMessage{UserID: message.UserID, Payload: "echo"}
	})

	party.UserInvalidMessageHandler = func(party *sockparty.Party, user *sockparty.User, message sockparty.IncomingMessage) {
		panic(fmt.Errorf("Test: Invalid message"))
	}

	// Spawn server
	stop := make(chan bool)
	go basicPartyServer(party, stop)
	go party.Listen()
	defer func() {
		party.StopListening <- true
		stop <- true
	}()

	connCount := 10
	messagesPerConn := 200
	messagesReceived := bully(connCount, messagesPerConn)
	// Assert that got as many messages as connections were echoed back
	if messagesReceived != connCount*messagesPerConn {
		log.Fatalf("Expected to recieve %d messages but got %d", connCount*messagesPerConn, messagesReceived)
	}
}
