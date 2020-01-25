# SockParty

SockParty is a WebSocket room manager for Golang.

##### `go get github.com/izzymg/sockparty`

## Built With

* [nhooyr/websocket](https://github.com/nhooyr/websocket)

## Example

#### [Full chat room example package](/example)

#### Summary

```go

// Generate a new party
party := sockparty.NewParty("A new room", &sockparty.Options{
	PingTimeout:	time.Second * 10,
        PingFrequency: 	time.Second * 20,
	RateLimiter:   	rate.NewLimiter(rate.Every(time.Millisecond*100), 5),
})

// Register your own message events and handle them as you wish.
party.SetMessageEvent("chat_message", func(party *sockparty.Party, message sockparty.IncomingMessage) {
        json.Unmarshal(message.Payload, ...)
	
	// Channel based API
	party.SendMessage <- sockparty.OutgoingMessage{
		Broadcast: true,
		// ...
	}
})

// Respond to users joining, leaving, etc
party.UserAddedHandler(...)

// Start the party
go party.Listen()

// Party implements http.Handler, will upgrade requests to WebSocket
server.Get("/join", party)


party.StopListening <- true
```

## Contributing

Submit a PR~


### TODO:
* Rewrite tests
* Add authentication function to parties
* Investigate pooling message buffers
