# SockParty 💬
[![Build status](https://api.travis-ci.org/izzymg/sockparty.svg?branch=master)](https://github.com/izzymg/releases)
[![codecov](https://codecov.io/gh/izzymg/sockparty/branch/master/graph/badge.svg)](https://codecov.io/gh/izzymg/sockparty)

SockParty is a WebSocket room manager for Golang. Allows creation of rooms, which manage WebSocket connections and an API to communicate with them.

It is currently in development, and may undergo breaking API changes.

# Install

`go get github.com/izzymg/sockparty`

## About

On usescases, sockparty was built to provide a higher-level API for a WebSocket based chat-room and media player

* JSON based messages
* Channel messages to any or all users in a party
* Simply register a party as an HTTP handler to allow users to join

## Example:

See [full chat room example](/example) package

## Built With

* [nhooyr/websocket](https://github.com/nhooyr/websocket)

## Contributing

Submit a PR~
