package client

import (
	"context"
	"fmt"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type event struct {
	Event   string      `json:"event"`
	Payload interface{} `json:"payload"`
}

type message struct {
	Message string `json:"message"`
}

// DoClient is a quick test implementation
func DoClient() {

	spam := &event{
		Event: "chat_message",
		Payload: message{
			Message: "Spam!!!",
		},
	}

	// Cancelable context with a 1min timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, "ws://localhost:3500", nil)
	if err != nil {
		panic(fmt.Errorf("Websocket dial failed: %v", err))
	}
	defer conn.Close(websocket.StatusNormalClosure, "Bye!")

	if err != nil {
		panic(fmt.Errorf("WS JSON write failed: %v", err))
	}

	err = wsjson.Write(ctx, conn, spam)
	if err != nil {
		fmt.Println(fmt.Errorf("Write fail: %w", err))
	}

	err = wsjson.Write(ctx, conn, spam)
	if err != nil {
		fmt.Println(fmt.Errorf("Write fail: %w", err))
	}

	err = wsjson.Write(ctx, conn, spam)
	if err != nil {
		fmt.Println(fmt.Errorf("Write fail: %w", err))
	}
}
