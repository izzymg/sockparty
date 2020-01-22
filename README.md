# Sockparty

Sockparty is a test implementation of a websocket chat-room with Golang and [nhooyr/websocket](https://github.com/nhooyr/websocket)

### Example

```go
party := sockparty.NewParty("A new room", &sockparty.Options{
	RateLimiter:   rate.NewLimiter(rate.Every(time.Millisecond*100), 5),
	AllowedOrigin: "http://localhost:80",
})

go party.Listen()

// Party implements http.Handler, will upgrade requests to WebSocket
router.Get("/newRoom", party)

fmt.Println(party.GetConnectedUserCount())

party.Stop <- true
```

### TODO:
* Message destination
* Add ready message
* Add join name message
* Add adapter for logging events, instead of stdout
* Better API control over channel buffer size
* Dedicated message structure and event emitting
* Add authentication function to parties
fmt.Println(party.GetConnectedUserCount())

party.Stop <- true
```
