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
	partyIncoming := make(chan sockparty.Incoming)
	party := sockparty.NewParty("Party", partyIncoming, sockparty.DefaultOptions())

	// Broadcast users joining.
	party.UserAddedHandler = func(userID uuid.UUID) {
		fmt.Printf("User %s joined the party\n", userID)
		party.SendMessage(context.TODO(), &sockparty.Outgoing{
			Broadcast: true,
			Payload: chatMessage{
				Body: fmt.Sprintf("User %s joined the party", userID),
			},
		})
		fmt.Println("Sent")
	}

	// Broadcast users leaving.
	party.UserRemovedHandler = func(userID uuid.UUID) {
		fmt.Printf("User %s left the party\n", userID)
		party.SendMessage(context.TODO(), &sockparty.Outgoing{
			Broadcast: true,
			Payload: chatMessage{
				Body: fmt.Sprintf("User %s left the party", userID),
			},
		})
	}

	/* Simple chat, take each message, validate it, and broadcast it back out. */
	go func() {
		for {
			select {
			case message := <-partyIncoming:
				// Check message type
				if message.Event != "chat_message" {
					continue
				}
				// Unmarshal the JSON payload.
				parsedMessage := &chatMessage{}
				err := json.Unmarshal(message.Payload, parsedMessage)
				if err != nil {
					// Send invalid payload error
					party.SendMessage(context.TODO(), &sockparty.Outgoing{
						UserID:  message.UserID,
						Event:   "error",
						Payload: "Invalid JSON",
					})
				}

				// Broadcast the data back out to users.
				party.SendMessage(context.TODO(), &sockparty.Outgoing{
					Broadcast: true,
					Event:     "chat_message",
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

	// Cleanly close party
	go func() {
		<-time.After(time.Minute)
		party.End()
	}()
	fmt.Println(server.ListenAndServe())
}
