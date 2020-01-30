package sockparty_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"nhooyr.io/websocket/wsjson"

	"golang.org/x/sync/errgroup"
	"nhooyr.io/websocket"

	"github.com/google/uuid"
	"github.com/izzymg/sockparty"
)

type testGenerator func() (http.Handler, func(t *testing.T))

// Test party's serving of new users
func GenAddUser() (http.Handler, func(t *testing.T)) {

	incoming := make(chan sockparty.Incoming)
	join := make(chan uuid.UUID)
	leave := make(chan uuid.UUID)
	party := sockparty.New("", incoming, join, leave, noPing())

	return party, func(t *testing.T) {
		// Generic request
		t.Run("Bad HTTP request to join", func(t *testing.T) {
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

		// Proper WS request
		for i := 0; i < 3; i++ {
			t.Run("Good WebSocket request to join", func(t *testing.T) {

				// User added channel should be sent to after dial.
				count := 0
				ctx, cancel := context.WithCancel(context.Background())
				go func() {
					<-join
					count++
					cancel()
				}()

				// Dial, should be one connection
				_, cleanupConns := makeConns(1)
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

				// Wait for user removed handler to finish, should be zero users.
				cleanupConns()
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
}

// Creates a simple echo party server and tests it echos back correct random data.
func GenEchoServer() (http.Handler, func(t *testing.T)) {
	// Create a party to echo messages back
	incoming := make(chan sockparty.Incoming)
	party := sockparty.New("", incoming, nil, nil, noPing())

	return party, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
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

		closeReason := "Exiting"
		messageEvent := sockparty.Event("echo")

		numConns := 10
		conns, cleanup := makeConns(numConns)
		defer cleanup()
		payloads := makeGarbageStrings(numConns)

		for index, conn := range conns {
			recv := make(chan []byte)

			// Start reading any messages coming in
			go func(c *websocket.Conn) {
				_, data, err := c.Read(context.Background())
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
			}(conn)

			/* Write message from client to the party */
			marshalled, err := json.Marshal(&testMessage{
				Event:   string(messageEvent),
				Payload: payloads[index],
			})
			if err != nil {
				t.Fatal(err)
			}
			err = conn.Write(context.Background(), websocket.MessageText, marshalled)
			if err != nil {
				panic(err)
			}

			/* Expect to have it returned and echoed back */
			data := <-recv
			tm, err := newTestMessage(data)
			if err != nil {
				t.Fatal(err)
			}

			err = tm.equalToOutgoing(sockparty.Outgoing{messageEvent, payloads[index]})
			if err != nil {
				t.Fatal(err)
			}
		}
		cancel()
	}
}

// Test messaging a single user from the party.
func GenTestMessage() (http.Handler, func(t *testing.T)) {
	inc := make(chan sockparty.Incoming)
	join := make(chan uuid.UUID)
	party := sockparty.New("", inc, join, nil, noPing())

	return party, func(t *testing.T) {
		message := &sockparty.Outgoing{
			Event:   "ferret",
			Payload: "weasel",
		}

		var id uuid.UUID
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			id = <-join
			cancel()
		}()

		conns, cleanup := makeConns(1)
		defer cleanup()

		// Wait for join to give user ID
		<-ctx.Done()

		ctx = conns[0].CloseRead(context.Background())
		err := party.Message(ctx, id, message)
		if err != nil {
			t.Fatal(err)
		}

		// Wait for closeread to get message
		<-ctx.Done()
	}
}

// Test messaging to all users
func GenBroadcast() (http.Handler, func(t *testing.T)) {
	party := sockparty.New("", nil, nil, nil, noPing())

	return party, func(t *testing.T) {
		message := &sockparty.Outgoing{
			Event:   "cat",
			Payload: "{ ferret }",
		}

		// Open n connections
		conns, cleanup := makeConns(10)
		defer cleanup()

		// Wait for them to read in broadcast data
		var g errgroup.Group

		for _, conn := range conns {
			c := conn // Prevent closure race
			g.Go(func() error {
				recv := &testMessage{}
				err := wsjson.Read(context.Background(), c, recv)
				if err != nil {
					return err
				}
				if err := recv.equalToOutgoing(*message); err != nil {
					return err
				}
				return nil
			})
		}

		err := party.Broadcast(context.Background(), message)
		if err != nil {
			t.Fatal(err)
		}

		if err := g.Wait(); err != nil {
			t.Fatal(err)
		}
	}
}

/* Generates and runs end-to-end tests. These tests rely on an HTTP server
which is reused here. */
func TestEndToEnds(t *testing.T) {
	// Generated tests modify this handler
	var handler http.Handler

	server := http.Server{
		Addr: getAddr(),
		// Handler is called directly
		Handler: http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			if handler != nil {
				handler.ServeHTTP(rw, req)
			} else {
				t.Error("Nil handler passed to server")
			}
		}),
	}
	defer server.Shutdown(context.TODO())
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			t.Errorf("HTTP server error: %v", err)
		}
	}()

	// Map tests to their generator functions and run them
	var tests = map[string]testGenerator{
		"Add User":          GenAddUser,
		"Message user":      GenTestMessage,
		"Broadcast message": GenBroadcast,
		"Echo server":       GenEchoServer,
	}

	for name, genTest := range tests {
		testHandler, test := genTest()
		handler = testHandler
		t.Run(name, test)
	}
}

// Test messaging to an invalid user returns an error
func TestNoSuchUser(t *testing.T) {
	party := sockparty.New("", nil, nil, nil, noPing())
	err := party.Message(context.Background(), uuid.New(), &sockparty.Outgoing{})
	if err != sockparty.ErrNoSuchUser {
		t.Fatalf("Expected no such user, got: %v", err)
	}
}
