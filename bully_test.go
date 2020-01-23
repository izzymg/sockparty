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
		PingFrequency: time.Second * 10,
		PingTimeout:   time.Second * 5,
		DoPing:        false,
		RateLimiter:   rate.NewLimiter(rate.Every(time.Millisecond*1), 1),
	})

	party.SetMessageEvent("message", func(party *sockparty.Party, message sockparty.IncomingMessage) {
		party.SendMessage <- sockparty.OutgoingMessage{
			UserID:  message.UserID,
			Payload: "back at you",
		}
	})

	party.UserInvalidMessageHandler = func(message sockparty.IncomingMessage) {
		panic(message)
	}

	go party.Listen()

	// Create HTTP server
	server := http.Server{
		Addr:    "localhost:3500",
		Handler: party,
	}

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			panic(err)
		}
	}()

	<-time.After(time.Second * 2)

	// Setup connection vars
	var closeFunctions []func()
	got := make(chan int, connectionCount)
	messagesReceived := 0

	for i := 0; i < connectionCount; i++ {
		closeFunc := spawn(messagesPerConnection, got)
		closeFunctions = append(closeFunctions, closeFunc)
	}

	if len(closeFunctions) != connectionCount {
		t.Fatal("Expected as many close functions as connections spawned")
	}

	// Start incrementing connections received
	going := true
	for going {
		select {
		case <-got:
			messagesReceived++
			fmt.Println(messagesReceived)
			if messagesReceived == connectionCount*messagesPerConnection {
				going = false
			}
		}
	}

	// Cleanup
	for _, close := range closeFunctions {
		close()
	}
	server.Shutdown(context.Background())

	// Assert that got as many messages as connections were echoed back
	if messagesReceived != connectionCount*messagesPerConnection {
		log.Fatalf("Expected to recieve %d messages but got %d", connectionCount*messagesPerConnection, messagesReceived)
	}
}
