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

	"golang.org/x/sync/errgroup"

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

// Random ID generator for users. This could come from a database for logins, etc.
func generateUID() (string, error) {
	uid, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return uid.String(), nil
}

// Test dialing, writing and closing off WebSocket connections.
func TestEndToEnd(t *testing.T) {

	var party *sockparty.Party

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
		party = sockparty.New(generateUID, &sockparty.Options{
			PingFrequency: 0,
		})
		t.Run(name, test)
	}
}

func TestJoinLeft(t *testing.T) {
	is := is.New(t)

	// Generate a testable ID
	bob := "bob"
	genID := func() (string, error) {
		return bob, nil
	}

	party := sockparty.New(genID,
		&sockparty.Options{
			PingFrequency: 0,
		},
	)

	userJoined := make(chan string)
	userLeft := make(chan string)
	party.RegisterOnUserJoined(userJoined)
	party.RegisterOnUserLeft(userLeft)

	// Dial a websocket connection, grab the ID
	d := wstest.NewDialer(party)
	c, _, err := d.Dial(addr, nil)
	is.NoErr(err)

	joinID := <-userJoined
	is.Equal(joinID, bob)

	// Close the connection, grab the ID again
	c.Close()

	leftID := <-userLeft
	is.Equal(leftID, bob)
}

// Test messaging a single user after fetching their ID on join.
func TestPartyMessage(t *testing.T) {
	is := is.New(t)

	party := sockparty.New(generateUID,
		&sockparty.Options{
			PingFrequency: 0,
		},
	)
	// Hook into user joins, incoming messages
	userJoined := make(chan string)
	party.RegisterOnUserJoined(userJoined)

	/* Dial a single connection to the party,
	and run its read method to avoid blocking. */
	d := wstest.NewDialer(party)
	c, _, err := d.Dial(addr, nil)
	go c.ReadMessage()
	is.NoErr(err)
	defer c.Close()

	joinID := <-userJoined
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	err = party.Message(ctx, joinID, &sockparty.Outgoing{
		Event:   "test_event",
		Payload: "Hello",
	})
	is.NoErr(err)
}

// Test messages received from clients come through the incoming channel.
func TestIncomingMessage(t *testing.T) {
	is := is.New(t)

	party := sockparty.New(generateUID,
		&sockparty.Options{
			PingFrequency: 0,
		},
	)
	incoming := make(chan sockparty.Incoming)
	party.RegisterIncoming(incoming)

	// Spawn connections after incoming channel is registered.
	connectionCount := 3
	conns, cleanup, err := makeConnections(connectionCount, party)
	is.NoErr(err)
	defer cleanup()

	var wg sync.WaitGroup
	wg.Add(connectionCount)

	go func() {
		for i := 0; i < connectionCount; i++ {
			<-incoming
			wg.Done()
		}
	}()

	testEvent := "testEvent"
	testPayload := "comfy_message_payload"
	testMessage := struct {
		event   string
		payload interface{}
	}{testEvent, testPayload}

	for _, conn := range conns {
		err := conn.WriteJSON(&testMessage)
		is.NoErr(err)
	}

	wg.Wait()
}

/*
GenBroadcaster generates a test broadcasting n messages to n users,
ensuring all connected websocket clients receive exactly those messages.
*/
func GenBroadcaster(connCount int, msgCount int) func(t *testing.T) {
	return func(t *testing.T) {
		is := is.New(t)
		party := sockparty.New(generateUID, &sockparty.Options{
			PingFrequency: 0,
		})

		// Make n conns
		conns, cleanup, err := makeConnections(connCount, party)
		is.NoErr(err)
		defer cleanup()

		var received uint32
		var errg errgroup.Group
		// For n connections, read n messages
		for ci, conn := range conns {
			conn := conn
			ci := ci
			errg.Go(func() error {
				for i := 0; i < msgCount; i++ {
					_, _, err := conn.ReadMessage()
					if err != nil {
						return err
					}
					atomic.AddUint32(&received, 1)
				}
				// Set done when read all messages
				fmt.Printf("Conn %d done\n", ci)
				return nil
			})
		}

		// Broadcast n messages
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
		defer cancel()
		for i := 0; i < msgCount; i++ {
			err := party.Broadcast(ctx, &sockparty.Outgoing{
				Event:   "msg",
				Payload: "broadcasting~",
			})
			if err != nil {
				t.Fatal(err)
			}
			fmt.Printf("Broadcast: %d\n", i)
		}

		fmt.Printf("About to wait\n")
		errg.Wait()
		is.Equal(received, uint32(connCount*msgCount))
	}
}

func TestBroadcast(t *testing.T) {
	var tests = map[string]func(t *testing.T){
		"2, 5": GenBroadcaster(2, 5),
		//"4, 2":  GenBroadcaster(4, 2),
		//"3, 30": GenBroadcaster(3, 30),
		"1, 8": GenBroadcaster(1, 8),
	}

	for name, test := range tests {
		t.Run(name, test)
	}
}

// Test that a user's ID can be looked up on join.
func TestUserExists(t *testing.T) {

	is := is.New(t)
	party := sockparty.New(generateUID, &sockparty.Options{
		PingFrequency: 0,
	})

	// Register a channel to listen for user joins, fetch the user when they've joined.
	onJoin := make(chan string)
	party.RegisterOnUserJoined(onJoin)

	_, cleanup, err := makeConnections(1, party)
	is.NoErr(err)
	defer cleanup()

	id := <-onJoin
	is.True(party.UserExists(id))
	is.True(!party.UserExists("idontexist"))
	is.True(party.GetConnectedUserCount() == 1)
}

// Test fetching a set of user's IDs
func TestGetUserIDs(t *testing.T) {
	is := is.New(t)

	party := sockparty.New(generateUID,
		&sockparty.Options{
			PingFrequency: 0,
		},
	)

	// Register a user join channel before joining n times
	userJoin := make(chan string)
	party.RegisterOnUserJoined(userJoin)

	userCount := 5
	_, cleanup, err := makeConnections(userCount, party)
	is.NoErr(err)
	defer cleanup()

	// Collect all user's IDs
	var userIDs []string
	for i := 0; i < userCount; i++ {
		id := <-userJoin
		userIDs = append(userIDs, id)
	}

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
