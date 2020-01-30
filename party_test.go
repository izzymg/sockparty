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

// Test party's serving of new users
func TestAddUser(t *testing.T) {

	incoming := make(chan sockparty.Incoming)
	join := make(chan uuid.UUID)
	leave := make(chan uuid.UUID)
	party, cleanup := testPServer(incoming, join, leave, noPing())

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

	for i := 0; i < 4; i++ {
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
	cleanup()
}

// Creates a simple echo party server and tests it echos back correct random data.
func TestMessageEach(t *testing.T) {
	// Create a party to echo messages back
	incoming := make(chan sockparty.Incoming)
	party, cleanup := testPServer(incoming, nil, nil, noPing())
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for {
			select {
			case <-ctx.Done():
				cleanup()
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
	conns, _ := makeConns(numConns)
	payloads := makeGarbageStrings(numConns)

	for index, conn := range conns {
		recv := make(chan []byte)

		// Start reading any messages coming in
		go func(c *websocket.Conn) {
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

	cancel() // Cleanup goroutine
}

func TestMessage(t *testing.T) {
	message := &sockparty.Outgoing{
		Event:   "ferret",
		Payload: "weasel",
	}
	inc := make(chan sockparty.Incoming)
	join := make(chan uuid.UUID)
	party, cleanup := testPServer(inc, join, nil, noPing())
	defer cleanup()

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

// Test messaging to an invalid user returns an error
func TestNoSuchUser(t *testing.T) {
	party := sockparty.New("", nil, nil, nil, noPing())
	err := party.Message(context.Background(), uuid.New(), &sockparty.Outgoing{})
	if err != sockparty.ErrNoSuchUser {
		t.Fatalf("Expected no such user, got: %v", err)
	}
}

// Test messaging to all users
func TestBroadcast(t *testing.T) {
	message := &sockparty.Outgoing{
		Event:   "cat",
		Payload: "{ ferret }",
	}

	party, cleanup := testPServer(nil, nil, nil, noPing())
	defer cleanup()

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
