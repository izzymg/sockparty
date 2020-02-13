// Package sockparty_test implements testing for SockParty
package sockparty_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/matryer/is"

	"github.com/izzymg/sockparty"
	"github.com/posener/wstest"
)

type TestMessage struct {
	Body string `json:"body"`
}

const addr = "ws://localhost:3000"

func TestEndToEnd(t *testing.T) {

	var party *sockparty.Party
	var incoming chan sockparty.Incoming

	is := is.New(t)

	var tests = map[string]func(t *testing.T){
		"Dial": func(t *testing.T) {
			d := wstest.NewDialer(party)
			c, resp, err := d.Dial(addr, nil)
			is.NoErr(err)
			defer c.Close()

			if resp.StatusCode != http.StatusSwitchingProtocols {
				t.Fatalf("Expected status 101, got %d", resp.StatusCode)
			}
		},

		"Send": func(t *testing.T) {
			d := wstest.NewDialer(party)
			c, _, err := d.Dial(addr, nil)
			is.NoErr(err)
			defer c.Close()

			err = c.WriteJSON(&TestMessage{"Echo"})
			is.NoErr(err)
		},

		"End": func(t *testing.T) {
			d := wstest.NewDialer(party)
			c, _, err := d.Dial(addr, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			party.End("Bye")
			is.Equal(party.GetConnectedUserCount(), 0)
		},
	}

	for name, test := range tests {
		incoming = make(chan sockparty.Incoming)
		party = sockparty.New(generateUID, incoming, &sockparty.Options{
			PingFrequency: 0,
		})
		t.Run(name, test)
	}
}

func TestJoin(t *testing.T) {
	is := is.New(t)

	// Generate a testable ID
	bob := "bob"
	genID := func() (string, error) {
		return bob, nil
	}

	done := make(chan bool)
	onJoin := func(id string) {
		is.Equal(id, bob)
		close(done)
	}

	incoming := make(chan sockparty.Incoming)
	party := sockparty.New(genID, incoming, &sockparty.Options{
		PingFrequency:   0,
		UserJoinHandler: onJoin,
	})

	d := wstest.NewDialer(party)
	c, _, err := d.Dial(addr, nil)
	is.NoErr(err)
	defer c.Close()

	// Wait for join
	select {
	case <-done:
		return
	case <-time.After(time.Second * 3):
		t.Fatal("Timeout waiting for user join event")
	}
}

func TestPartyMessage(t *testing.T) {
	is := is.New(t)

	getID := make(chan string)
	onJoin := func(id string) {
		// dont block
		go func() {
			getID <- id
		}()
	}

	incoming := make(chan sockparty.Incoming)
	party := sockparty.New(generateUID, incoming, &sockparty.Options{
		PingFrequency:   0,
		UserJoinHandler: onJoin,
	})

	d := wstest.NewDialer(party)
	c, _, err := d.Dial(addr, nil)
	// Call read message so
	go c.ReadMessage()
	is.NoErr(err)
	defer c.Close()

	select {
	case id := <-getID:
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
		defer cancel()
		err := party.Message(ctx, id, &sockparty.Outgoing{
			Event:   "msg",
			Payload: "Hello",
		})
		is.NoErr(err)
		return
	case <-time.After(time.Second * 5):
		t.Fatal("Timeout waiting for ID")
	}
}
