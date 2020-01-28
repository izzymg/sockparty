# SockParty 💬

SockParty is a WebSocket room manager for Golang. Allows creation of rooms, which manage WebSocket connections and an API to communicate with them.

It is currently in development, and may undergo breaking API changes.

##### `go get github.com/izzymg/sockparty`

## Built With

* [nhooyr/websocket](https://github.com/nhooyr/websocket)

## Features

On usescases, sockparty was built to provide a higher-level API for a WebSocket based chat-room and media player

* Register "types" for incoming messages and provide your own decoding, parsing & response handler.
* Channel messages to any or all users in a party
* Built purely using JSON
* Simply register a party as an HTTP handler to allow users to join

## Example:

See [full chat room example](/example) package

## Contributing

Submit a PR~


### TODO:
* Max context timeouts
* Channel based add/remove callbacks
* Further tests/benchmarks
* Restructure package layout