package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof"

	"github.com/izzymg/sockparty"
)

/* Chat room example with SockParty */

func main() {

	go func() {
		fmt.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	type ChatMessage struct {
		Body string `json:"body"`
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
		cm := &ChatMessage{}

		// Unmarshal payload into expected data format
		err := json.Unmarshal(message.Payload, cm)
		if err != nil {
			fmt.Println(err)
			return
		}

		party.SendMessage <- sockparty.OutgoingMessage{
			Event:     "chat_message",
			Broadcast: true,
			Payload:   cm,
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
