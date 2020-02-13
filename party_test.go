// Package sockparty_test implements testing for SockParty
package sockparty_test

import (
	"net/http"
	"testing"

	"github.com/matryer/is"

	"github.com/izzymg/sockparty"
	"github.com/posener/wstest"
)

type TestMessage struct {
	Body string `json:"body"`
}

const addr = "ws://localhost:3000"

func TestEndToEnd(t *testing.T) {
	incoming := make(chan sockparty.Incoming)
	party := sockparty.New(generateUID, incoming, &sockparty.Options{
		PingFrequency:    0,
		AllowCrossOrigin: true,
	})

	is := is.New(t)

	t.Run("Dial", func(t *testing.T) {
		d := wstest.NewDialer(party)
		c, resp, err := d.Dial(addr, nil)
		defer c.Close()
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusSwitchingProtocols {
			t.Fatalf("Expected status 101, got %d", resp.StatusCode)
		}
	})

	t.Run("Send", func(t *testing.T) {
		d := wstest.NewDialer(party)
		c, _, err := d.Dial(addr, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()

		err = c.WriteJSON(&TestMessage{"Echo"})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("End", func(t *testing.T) {
		d := wstest.NewDialer(party)
		c, _, err := d.Dial(addr, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		is.True(party.GetConnectedUserCount() > 0)
		party.End("Bye")
		is.Equal(party.GetConnectedUserCount(), 0)
	})
}
