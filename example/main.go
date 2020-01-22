package main

import (
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

		ChatMessageHandler: func(party *sockparty.Party, message *sockparty.Message) {
			// Broadcast each chat message received back to the clients.
			// You may want to do any amount of processing and validation here.
			fmt.Println("Got message")
			party.Broadcast <- *message
		},
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
