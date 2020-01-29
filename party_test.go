package sockparty_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"nhooyr.io/websocket"

	"golang.org/x/time/rate"

	"github.com/google/uuid"
	"github.com/izzymg/sockparty"
)

/*
	Add users
*/

// Default opts for testing
func noPing() *sockparty.Options {
	return &sockparty.Options{
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

	incoming := make(chan sockparty.Incoming)
	join := make(chan uuid.UUID)
	leave := make(chan uuid.UUID)
	party := sockparty.New("Party", incoming, join, leave, noPing())

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

			// User added channel should be sent to after dial.
			count := 0
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				<-join
				count++
				cancel()
			}()

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
			go func() {
				<-leave
				cancel()
			}()
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

// Make n conns, return cleanup
func makeConns(n int) ([]*websocket.Conn, func()) {
	var conns []*websocket.Conn
	for i := 0; i < n; i++ {
		conn, _, err := websocket.Dial(context.Background(), wsAddr(), nil)
		if err != nil {
			panic(err)
		}
		conns = append(conns, conn)
	}

	return conns, func() {
		for _, conn := range conns {
			conn.Close(websocket.StatusNormalClosure, "Exit")
		}
	}
}

func makeGarbageStrings(n int) []string {
	rand.Seed(time.Now().UnixNano())
	var ret []string
	for i := 0; i < n; i++ {
		ret = append(ret, fmt.Sprintf("%x", rand.Intn(600)))
	}
	return ret
}

// Creates a simple echo party server and tests it echos back correct random data.
func TestMessageEach(t *testing.T) {
	// Create a party to echo messages back
	incoming := make(chan sockparty.Incoming)
	party := sockparty.New("Party", incoming, nil, nil, noPing())
	go func() {
		for {
			select {
			case message, ok := <-incoming:
				if !ok {
					panic("Incoming not ok")
				}
				// Echo back
				party.Message(context.Background(), message.UserID, &sockparty.Outgoing{
					Event:   message.Event,
					Payload: message.Payload,
				})
			}
		}
	}()

	defer tServer(party)()

	type msgType struct {
		Event   string      `json:"event"`
		Payload interface{} `json:"payload"`
	}

	numConns := 10
	closeReason := "Exiting"
	ev := "echo"
	conns, _ := makeConns(numConns)
	//defer cleanup()
	payloads := makeGarbageStrings(numConns)

	for index, conn := range conns {
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
		marshalled, err := json.Marshal(&msgType{
			Event:   ev,
			Payload: payloads[index],
		})
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

		if unmarshalled.Event != ev {
			t.Fatalf("Expected event %s, got %s", ev, unmarshalled.Event)
		}

		if unmarshalled.Payload != payloads[index] {
			t.Fatalf("Expected payload %s, got %s", payloads[index], unmarshalled.Payload)
		}
	}

}

func TestMessage(t *testing.T) {
	inc := make(chan sockparty.Incoming)
	join := make(chan uuid.UUID)
	party := sockparty.New("", inc, join, nil, noPing())

	defer tServer(party)()

	var id uuid.UUID

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		id = <-join
		cancel()
	}()

	conn, _, err := websocket.Dial(context.Background(), wsAddr(), nil)
	defer conn.Close(websocket.StatusNormalClosure, "")
	if err != nil {
		t.Fatal(err)
	}

	// Wait for join to give user ID
	<-ctx.Done()

	ctx = conn.CloseRead(context.Background())
	err = party.Message(ctx, id, &sockparty.Outgoing{
		Event:   "test",
		Payload: "Payload",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for closeread to get message
	<-ctx.Done()
}

func TestNoSuchUser(t *testing.T) {
	party := sockparty.New("", nil, nil, nil, noPing())
	err := party.Message(context.Background(), uuid.New(), &sockparty.Outgoing{})
	if err != sockparty.ErrNoSuchUser {
		t.Fatalf("Expected no such user, got: %v", err)
	}
}
