package sockparty_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"nhooyr.io/websocket"

	"golang.org/x/time/rate"

	"github.com/izzymg/sockparty"
	"github.com/izzymg/sockparty/sockmessages"
	"github.com/izzymg/sockparty/sockoptions"
)

/*
	Add users
*/

// Default opts for testing
func noPing() *sockoptions.Options {
	return &sockoptions.Options{
		AllowCrossOrigin: true,
		RateLimiter:      rate.NewLimiter(rate.Inf, 1),
		PingFrequency:    0,
		PingTimeout:      0,
	}
}

// Returns an address to use for testing, with no protocol
func getAddr() string {
	if addr, ok := os.LookupEnv("TEST_ADDR"); ok {
		return addr
	}
	return "localhost:3000"
}

func wsAddr() string {
	return fmt.Sprintf("ws://%s", getAddr())
}

// Creates an HTTP server for testing, returns cleanup function.
func tServer(handler http.Handler) func() {
	server := http.Server{
		Addr:    getAddr(),
		Handler: handler,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	return func() {
		server.Shutdown(context.Background())
	}
}

// Test party's serving of new users
func TestAddUser(t *testing.T) {
	incoming := make(chan sockmessages.Incoming)
	party := sockparty.NewParty("Party", incoming, noPing())
	defer tServer(party)()

	// Generic request
	t.Run("Fail", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req, err := http.NewRequest("GET", "/", nil)
		if err != nil {
			t.Fatal(err)
		}

		party.ServeHTTP(rr, req)
		if rr.Code != http.StatusBadRequest {
			t.Fatalf("Expected bad request, got %d", rr.Code)
		}
		if count := party.GetConnectedUserCount(); count != 0 {
			t.Fatalf("Expected %d connected, got %d", 0, count)
		}
	})

	for i := 0; i < 10; i++ {
		// Proper WS request
		t.Run("OK", func(t *testing.T) {

			// User added handler should be invoked after dial.
			count := 0
			ctx, cancel := context.WithCancel(context.Background())
			party.UserAddedHandler = func(userID string) {
				count++
				cancel()
			}

			// Perform dial, want successful connection
			conn, _, err := websocket.Dial(ctx, wsAddr(), nil)
			if err != nil {
				t.Fatal(err)
			}

			// Wait for user added handler to finish, should be 1 user
			<-ctx.Done()
			if count != 1 {
				t.Fatalf("Expected %d connected, got %d", 1, count)
			}

			// User removed should be invoked after close.
			ctx, cancel = context.WithCancel(context.Background())
			party.UserRemovedHandler = func(userID string) {
				cancel()
			}
			// Perform close, want success
			err = conn.Close(websocket.StatusNormalClosure, "")
			if err != nil {
				t.Fatal(err)
			}
			// Wait for user removed handler to finish, should be zero users.
			<-ctx.Done()
			c := party.GetConnectedUserCount()
			if c != 0 {
				t.Fatalf("Expected %d connected, got %d", 0, c)
			}
		})
	}

	// Sanity check
	if count := party.GetConnectedUserCount(); count != 0 {
		t.Fatalf("Expected %d connected, got %d", 0, count)
	}
}

// Creates a simple echo party server and tests it.
func TestMessageEach(t *testing.T) {
	// Create a party to echo messages back
	incoming := make(chan sockmessages.Incoming)
	party := sockparty.NewParty("Party", incoming, noPing())
	defer tServer(party)()

	go func() {
		for {
			select {
			case message, ok := <-incoming:
				if !ok {
					panic("Incoming not ok")
				}
				// Echo back
				party.SendMessage(context.Background(), &sockmessages.Outgoing{
					Event:   message.Event,
					UserID:  message.UserID,
					Payload: message.Payload,
				})
			}
		}
	}()

	type msgType struct {
		Event   string      `json:"event"`
		Payload interface{} `json:"payload"`
	}
	msg := &msgType{
		Event:   "hello",
		Payload: "hey",
	}

	closeReason := "Exiting"
	conn, _, err := websocket.Dial(
		context.Background(),
		wsAddr(),
		nil,
	)
	defer conn.Close(websocket.StatusNormalClosure, closeReason)
	if err != nil {
		t.Fatal(err)
	}

	recv := make(chan []byte)

	// Start reading any messages coming in
	go func() {
		_, data, err := conn.Read(context.Background())
		var ce websocket.CloseError
		if err != nil {
			// Expected close message with correct reason.
			if errors.As(err, &ce) && ce.Reason == closeReason {
				close(recv)
				return
			}
			panic(err)
		}
		recv <- data
	}()

	// Write message out
	marshalled, err := json.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	err = conn.Write(context.Background(), websocket.MessageText, marshalled)
	if err != nil {
		panic(err)
	}

	// Expect it echoed bacl
	data := <-recv
	unmarshalled := &msgType{}
	err = json.Unmarshal(data, unmarshalled)
	if err != nil {
		t.Fatal(err)
	}

	if unmarshalled.Event != msg.Event {
		t.Fatalf("Expected event %s, got %s", msg.Event, unmarshalled.Event)
	}

	if unmarshalled.Payload != msg.Payload {
		t.Fatalf("Expected payload %s, got %s", msg.Payload, unmarshalled.Payload)
	}

}
