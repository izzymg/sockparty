package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/izzymg/sockparty"
	"github.com/pkg/profile"
)

/* Chat room example with SockParty */

func main() {

	defer profile.Start(profile.MemProfile, profile.ProfilePath(".")).Stop()

	type ChatMessage struct {
		Payload struct {
			Message string `json:"message"`
		}
	}

	type ErrOut struct {
		Err string `json:"err"`
	}

	// Setup a new party with a rate limiter and allowed client origin
	party := sockparty.NewParty("Party", sockparty.DefaultOptions())
	// Add some handlers

	party.UserAddedHandler = func(ID string, name string) {
		fmt.Println("Log: user joined the party")
	}
	party.UserRemovedHandler = func(ID string, name string) {
		fmt.Println("Log: user removed from party")
	}

	party.UserInvalidMessageHandler = func(message sockparty.IncomingMessage) {
		fmt.Println("Log: user sent an invalid message")
		party.SendMessage <- sockparty.OutgoingMessage{
			Event:   "error",
			Payload: &ErrOut{Err: "Invalid message event received"},
			UserID:  message.UserID,
		}
	}

	/* Set event handlers. When a JSON message is sent with the format of { "event": "your_event" },
	the handler with the corresponding event name will be triggered */
	party.SetMessageEvent("chat_message", func(party *sockparty.Party, message sockparty.IncomingMessage) {

		// Decode the payload byte slice into the expected data format
		data := &ChatMessage{}
		err := json.Unmarshal(message.Payload, data)
		if err != nil {
			party.SendMessage <- sockparty.OutgoingMessage{
				Event:   "error",
				Payload: &ErrOut{Err: "Failed to parse chat message JSON"},
			}
			return
		}

		// Some validation logic...

		// Broadcast it back to the users, making sure the payload struct only contains the data.
		party.SendMessage <- sockparty.OutgoingMessage{
			Broadcast: true,
			Event:     "chat_message",
			Payload:   data.Payload,
		}
	})

	// Instruct the party to listen on its channels and defer a stop command.
	// Listen blocks.
	go party.Listen()
	defer func() {
		party.StopListening <- true
	}()

	server := http.Server{
		Addr: "localhost:3000",
		/* Party implements http.Handler and will treat requests
		as a new user joining the party by upgrading them to WebSocket. */
		Handler: party,
	}

	server.ListenAndServe()
}
