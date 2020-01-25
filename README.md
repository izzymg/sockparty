# SockParty

SockParty is a WebSocket chat room manager for Golang.

##### `go get github.com/izzymg/sockparty`

## Built With

* [nhooyr/websocket](https://github.com/nhooyr/websocket)

## Example

```go
party := sockparty.NewParty("A new room", &sockparty.Options{
	RateLimiter:   rate.NewLimiter(rate.Every(time.Millisecond*100), 5),
})

party.SetMessageEvent("chat_message", func(party *sockparty.Party, message sockparty.IncomingMessage) {
	// Handle message
	party.SendMessage <- sockparty.OutgoingMessage{
		Broadcast: true,
		// ...
	}
})

go party.Listen()

// Party implements http.Handler, will upgrade requests to WebSocket
router.Get("/newRoom", party)

fmt.Println(party.GetConnectedUserCount())

party.StopListening <- true
```

## Contributing

Submit a PR~


### TODO:
* Rewrite tests
* Add authentication function to parties
* Investigate pooling message buffers
