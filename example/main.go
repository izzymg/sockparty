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
	Party    *sockparty.Party
	Incoming chan sockparty.Incoming
	Joined   chan string
	Leave    chan string
}

// Run begins handling incoming messages, blocking.
func (ca *ChatApp) Run(ctx context.Context) {
	for {
		select {
		// Exit when context expires
		case <-ctx.Done():
			fmt.Println("Bye!")
			return

		// Broadcast user joins, and send a welcome message
		case userID := <-ca.Joined:
			// No need to pass a struct in
			ca.Party.Message(ctx, userID, &sockparty.Outgoing{
				Event:   "welcome",
				Payload: "Welcome!",
			})
			ca.Party.Broadcast(ctx, &sockparty.Outgoing{
				Event:   "user_join",
				Payload: fmt.Sprintf("User %q joined", userID),
			})

		// Broadcast user leaves
		case userID := <-ca.Leave:
			ca.Party.Broadcast(ctx, &sockparty.Outgoing{
				Event:   "user_leave",
				Payload: fmt.Sprintf("User %q left", userID),
			})
		// Broadcast chat messages back to users after validating.
		case message := <-ca.Incoming:
			// Check message type
			if message.Event != "chat_message" {
				continue
			}
			// Unmarshal the JSON payload.
			parsedMessage := &ChatMessage{}
			err := json.Unmarshal(message.Payload, parsedMessage)
			if err != nil {
				// Send invalid payload error
				ca.Party.Message(ctx, message.UserID, &sockparty.Outgoing{
					Event:   "error",
					Payload: "Invalid JSON",
				})
			}

			// Broadcast the data back out to users, sanitized for a web app.
			ca.Party.Broadcast(ctx, &sockparty.Outgoing{
				Event: "chat_message",
				Payload: ChatMessage{
					Body: html.EscapeString(parsedMessage.Body),
				},
			})
		}
	}
}

/*
This is an example chat application using SockParty. It takes messages from users,
parses them as JSON, and broadcasts them back to users.
*/

func main() {

	// Create a new sockparty, which implements http.Handler to upgrade requests to WebSocket.
	app := ChatApp{
		Party:    sockparty.New(generateUID, sockparty.DefaultOptions()),
		Incoming: make(chan sockparty.Incoming),
		Joined:   make(chan string),
		Leave:    make(chan string),
	}

	/* It's up to the consumer to make the channels, so you can configure buffer size, etc.
	If you don't register these channels, messages will simply be discarded, so make sure
	to register the incoming channel before allowing any connections. */
	app.Party.RegisterIncoming(app.Incoming)
	app.Party.RegisterOnUserJoined(app.Joined)
	app.Party.RegisterOnUserLeft(app.Leave)

	// Run the app for 2 minutes, then shut it down gracefully.
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*2)
	go app.Run(ctx)

	/* Party implements http.Handler and will treat requests as a new user
	joining the party by upgrading them to WebSocket. You could wrap this
	by calling ServeHTTP directly from within a handler, to verify incoming requests. */
	server := http.Server{
		Addr:    "localhost:3000",
		Handler: app.Party,
	}
	go server.ListenAndServe()

	// Block until the context times out.
	select {
	case <-ctx.Done():
		cancel()
		// You could also shut the party down with a nice message to connected users.
		app.Party.End("Party is over folks")
		server.Shutdown(ctx)
		return
	}
}
