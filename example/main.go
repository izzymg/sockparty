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

/* Chat room example with SockParty */

func main() {

	// Simple chat message
	type chatMessage struct {
		// Body of the chat message.
		Body string `json:"body"`
	}

	/* Setup a new party. All incoming messages from party users
	will be passed through the incoming channels. */
	incoming := make(chan sockparty.Incoming)
	joins := make(chan uuid.UUID)
	leaves := make(chan uuid.UUID)
	party := sockparty.New("Party", incoming, joins, leaves, sockparty.DefaultOptions())

	/* Simple chat, take each message, validate it, and broadcast it back out. */
	go func() {
		for {
			select {
			case id := <-joins:
				// Broadcast user joining
				fmt.Printf("User %s joined the party\n", id)
				party.Broadcast(context.TODO(), &sockparty.Outgoing{
					Payload: chatMessage{
						Body: fmt.Sprintf("User %s joined the party", id),
					},
				})
			case id := <-leaves:
				// Broadcast user leaving
				fmt.Printf("User %s left the party\n", id)
				party.Broadcast(context.TODO(), &sockparty.Outgoing{
					Payload: chatMessage{
						Body: fmt.Sprintf("User %s left the party", id),
					},
				})
			case message := <-incoming:
				// Check message type
				if message.Event != "chat_message" {
					continue
				}
				// Unmarshal the JSON payload.
				parsedMessage := &chatMessage{}
				err := json.Unmarshal(message.Payload, parsedMessage)
				if err != nil {
					// Send invalid payload error
					party.Message(context.TODO(), message.UserID, &sockparty.Outgoing{
						Event:   "error",
						Payload: "Invalid JSON",
					})
				}

				// Broadcast the data back out to users.
				party.Broadcast(context.TODO(), &sockparty.Outgoing{
					Event: "chat_message",
					Payload: chatMessage{
						Body: html.EscapeString(parsedMessage.Body),
					},
				})
			}
		}
	}()

	/* Party implements http.Handler and will treat requests as a new user
	joining the party by upgrading them to WebSocket. You could wrap this by calling
	ServeHTTP directly from within a handler. */
	server := http.Server{
		Addr:    "localhost:3000",
		Handler: party,
	}

	// Cleanly close party, attempting to gracefully close all connections with a normal status.
	go func() {
		<-time.After(time.Minute)
		party.End("Party's over folks")
	}()
	fmt.Println(server.ListenAndServe())
}
