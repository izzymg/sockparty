// Package main is an example chat app using SockParty.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/izzymg/sockparty"
)

// Random ID generator for users. This could come from a database for logins, etc.
func generateUID() (string, error) {
	uid, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return uid.String(), nil
}

// ChatMessage is a simple JSON chat message.
type ChatMessage struct {
	Body string `json:"body"`
}

// ChatApp is an example chat application built with Sockparty.
type ChatApp struct {
	Party *sockparty.Party
}

// Run begins handling incoming messages, blocking.
func (ca *ChatApp) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			// Exit when context expires
			fmt.Println("Bye!")
			return
		case message := <-ca.Party.Incoming:
			// Check message type
			if message.Event != "chat_message" {
				continue
			}
			// Unmarshal the JSON payload.
			parsedMessage := &ChatMessage{}
			err := json.Unmarshal(message.Payload, parsedMessage)
			if err != nil {
				// Send invalid payload error
				ca.Party.Message(context.TODO(), message.UserID, &sockparty.Outgoing{
					Event:   "error",
					Payload: "Invalid JSON",
				})
			}

			// Broadcast the data back out to users.
			ca.Party.Broadcast(context.TODO(), &sockparty.Outgoing{
				Event: "chat_message",
				Payload: ChatMessage{
					Body: html.EscapeString(parsedMessage.Body),
				},
			})
		}
	}
}

// OnUserJoin broadcasts user join notifications
func (ca *ChatApp) OnUserJoin(id string) {
	// Broadcast user joining
	fmt.Printf("User %s joined the party, connected %d\n", id, ca.Party.GetConnectedUserCount())
	ca.Party.Broadcast(context.Background(), &sockparty.Outgoing{
		Payload: ChatMessage{
			Body: fmt.Sprintf("User %s joined the party", id),
		},
	})
}

// OnUserLeave broadcasts user leave notifications
func (ca *ChatApp) OnUserLeave(id string) {
	// Broadcast user joining
	fmt.Printf("User %s left the party, connected %d\n", id, ca.Party.GetConnectedUserCount())
	ca.Party.Broadcast(context.Background(), &sockparty.Outgoing{
		Payload: ChatMessage{
			Body: fmt.Sprintf("User %s left the party", id),
		},
	})
}

func main() {

	// It's up to the consumer to make the channel, so you can configure buffer size, etc.
	incoming := make(chan sockparty.Incoming)

	var app ChatApp
	// Create a new sockparty, which implements http.Handler to upgrade requests to WebSocket.
	party := sockparty.New(generateUID, incoming, &sockparty.Options{
		UserJoinHandler:  app.OnUserJoin,
		UserLeaveHandler: app.OnUserLeave,
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*2)
	app.Party = party
	go app.Run(ctx)

	/* Party implements http.Handler and will treat requests as a new user
	joining the party by upgrading them to WebSocket. You could wrap this by calling
	ServeHTTP directly from within a handler. */
	server := http.Server{
		Addr:    "localhost:3000",
		Handler: party,
	}
	go server.ListenAndServe()

	// Block until the context times out.
	select {
	case <-ctx.Done():
		cancel()
		party.End("Party is over folks")
		server.Shutdown(ctx)
		return
	}
}
