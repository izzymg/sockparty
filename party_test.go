package sockparty_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"nhooyr.io/websocket"
	"os"
	"testing"

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
	return "localhost:3400"
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
	party := sockparty.NewParty("Party", incoming, sockoptions.DefaultOptions())
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
	})

	// Proper WS request
	t.Run("OK", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://%s", getAddr()), nil)
		if err != nil {
			t.Fatal(err)
		}
	})
}