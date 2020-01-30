package sockparty_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/izzymg/sockparty"
	"golang.org/x/time/rate"
	"nhooyr.io/websocket"
)

// Used for testing the validity of messages received from the party.
type testMessage struct {
	Event   string `json:"event"`
	Payload string `json:"payload"`
}

// Returns error if the test message is not equal to the outgoing message.
func (tm *testMessage) equalToOutgoing(og sockparty.Outgoing) error {
	if tm.Event != string(og.Event) {
		return fmt.Errorf("Expected event %q to match %q", tm.Event, og.Event)
	}
	if tm.Payload != og.Payload {
		return fmt.Errorf("Expected payload %v to match %v", tm.Payload, og.Payload)
	}
	return nil
}

// Unmarshals data into a new test message
func newTestMessage(data []byte) (*testMessage, error) {
	tm := &testMessage{}
	err := json.Unmarshal(data, tm)
	if err != nil {
		return nil, err
	}
	return tm, nil
}

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

// Creates a new party & connects an HTTP server for testing, returns cleanup function.
func testPServer(inc chan sockparty.Incoming, join chan<- uuid.UUID, leave chan<- uuid.UUID, options *sockparty.Options) (*sockparty.Party, func()) {
	party := sockparty.New("", inc, join, leave, options)
	server := http.Server{
		Addr:    getAddr(),
		Handler: party,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	return party, func() {
		server.Shutdown(context.Background())
		party.End()
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
