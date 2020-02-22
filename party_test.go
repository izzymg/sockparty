// Package sockparty_test implements testing for SockParty
package sockparty_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/matryer/is"

	"github.com/izzymg/sockparty"
	"github.com/posener/wstest"
)

type TestMessage struct {
	Body string `json:"body"`
}

const addr = "ws://localhost:3000"

// Random ID generator for users. This could come from a database for logins, etc.
func generateUID() (string, error) {
	uid, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return uid.String(), nil
}

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
		getID <- id
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

func makeConnections(n int, h http.Handler) ([]*websocket.Conn, func(), error) {
	conns := make([]*websocket.Conn, n)
	cleanup := func() {
		for _, conn := range conns {
			if conn != nil {
				conn.Close()
			}
		}
	}

	for i := 0; i < n; i++ {
		d := wstest.NewDialer(h)
		<-time.After(time.Millisecond * 200)
		c, _, err := d.Dial(addr, nil)
		if err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("Failed to dial conn n:%d, %v", i, err)
		}
		conns[i] = c
	}
	return conns, cleanup, nil
}

func GenBroadcaster(connCount int, msgCount int) func(t *testing.T) {
	return func(t *testing.T) {
		is := is.New(t)
		incoming := make(chan sockparty.Incoming)
		party := sockparty.New(generateUID, incoming, &sockparty.Options{
			PingFrequency: 0,
		})

		// Make n conns
		conns, cleanup, err := makeConnections(connCount, party)
		is.NoErr(err)
		defer cleanup()

		var received uint32
		var wg sync.WaitGroup
		wg.Add(connCount)
		// For n connections, read n messages
		for _, conn := range conns {
			go func(c *websocket.Conn) {
				for i := 0; i < msgCount; i++ {
					c.ReadMessage()
					atomic.AddUint32(&received, 1)
				}
				// Set done when read all messages
				wg.Done()
			}(conn)
		}

		// Broadcast n messages
		for i := 0; i < msgCount; i++ {
			go party.Broadcast(context.Background(), &sockparty.Outgoing{
				Event:   "msg",
				Payload: "broadcasting~",
			})
		}

		wg.Wait()
		is.Equal(received, uint32(connCount*msgCount))
	}
}

func TestBroadcast(t *testing.T) {
	var tests = map[string]func(t *testing.T){
		"2, 5":  GenBroadcaster(2, 5),
		"4, 2":  GenBroadcaster(4, 2),
		"3, 30": GenBroadcaster(3, 30),
		"1, 8":  GenBroadcaster(1, 8),
	}

	for name, test := range tests {
		t.Run(name, test)
	}
}

func TestUserExists(t *testing.T) {

	is := is.New(t)
	var wg sync.WaitGroup
	wg.Add(1)

	opts := &sockparty.Options{
		PingFrequency: 0,
	}
	incoming := make(chan sockparty.Incoming)
	party := sockparty.New(generateUID, incoming, opts)
	opts.UserJoinHandler = func(id string) {
		is.True(party.UserExists(id))
		is.Equal(party.UserExists("idontexist"), false)
		wg.Done()
	}

	_, cleanup, err := makeConnections(1, party)
	is.NoErr(err)
	defer cleanup()
	wg.Wait()
}

func TestGetUserIDs(t *testing.T) {
	is := is.New(t)

	opts := &sockparty.Options{PingFrequency: 0}

	party := sockparty.New(generateUID, make(chan sockparty.Incoming), opts)

	// Collect a list of all joined users.
	var userIDs []string
	var mut sync.Mutex
	var wg sync.WaitGroup

	opts.UserJoinHandler = func(userID string) {
		// UserJoinHandler will be called from many goroutines.
		mut.Lock()
		defer mut.Unlock()
		defer wg.Done()
		userIDs = append(userIDs, userID)
	}

	// Add n users
	userCount := 5
	wg.Add(userCount)
	_, cleanup, err := makeConnections(userCount, party)
	is.NoErr(err)
	defer cleanup()

	// Wait for all users to join
	wg.Wait()

	fetchedIDs := party.GetConnectedUserIDs()

	// Ensure each ID in userIDs is contained in fetched IDs
	is.Equal(len(fetchedIDs), len(userIDs))
	for userId := range userIDs {
		found := false
		for fetchedID := range fetchedIDs {
			if userId == fetchedID {
				found = true
				break
			}
		}
		if !found {
			is.Fail()
		}
	}
}
