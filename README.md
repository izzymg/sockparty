# Sockparty

Sockparty is a test implementation of a websocket chat-room with Golang and [nhooyr/websocket](https://github.com/nhooyr/websocket)

### Example

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

### TODO:
* Add ready/received message
* Better API control over channel buffer size
* Add authentication function to parties
* Investigate pooling message buffers