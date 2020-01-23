package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/izzymg/sockparty"
	"golang.org/x/time/rate"
)

/* Chat room example with SockParty */

func main() {

	// Setup a new party with a rate limiter and allowed client origin
	party := sockparty.NewParty("Party", &sockparty.Options{
		AllowedOrigin: "http://localhost:80",
		RateLimiter:   rate.NewLimiter(rate.Every(time.Millisecond*100), 2),
	})

	/* Set event handlers. When a JSON message is sent with the format of { "event": "your_event" },
	the handler with the corresponding event name will be triggered */
	party.SetMessageEvent("chat_message", func(party *sockparty.Party, message sockparty.IncomingMessage) {

		// Decode the payload byte slice into the expected data format

		data := &struct {
			Payload struct {
				Message string `json:"message"`
			}
		}{}

		err := json.Unmarshal(message.Payload, data)
		if err != nil {
			fmt.Println(err)
		}

		if len(data.Payload.Message) > 200 || len(data.Payload.Message) < 1 {
			// Some validation logic
			return
		}
		party.Broadcast <- sockparty.OutgoingMessage{
			Event:   "chat_message",
			Payload: data,
		}
	})

	// Instruct the party to listen on its channels and defer a stop command.
	// Listen blocks.
	go party.Listen()
	defer func() {
		party.Stop <- true
	}()

	server := http.Server{
		Addr: "localhost:3000",
		/* Party implements http.Handler and will treat requests
		as a new user joining the party by upgrading them to WebSocket. */
		Handler: party,
	}

	server.ListenAndServe()
}
