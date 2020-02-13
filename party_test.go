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
			PingFrequency:    0,
			AllowCrossOrigin: true,
		})
		t.Run(name, test)
	}
}
