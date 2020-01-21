package party

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// Message is a struct needing documentation
type Message struct {
	Event   string      `json:"event"`
	Payload interface{} `json:"payload"`
}

// Create a new user from a websocket connection.
func newUser(name string, connection *websocket.Conn) (*User, error) {
	uid, err := uuid.NewUUID()
	if err != nil {
		return nil, fmt.Errorf("Failed to generate a UUID for a new user: %w", err)
	}
	return &User{
		Name:       name,
		ID:         uid.String(),
		connection: connection,
		to:         make(chan Message),
		from:       make(chan Message),
	}, nil
}

// User represents a websocket connection from a client.
type User struct {
	ID         string
	Name       string
	from       chan Message
	to         chan Message
	connection *websocket.Conn
}

// Close the channels and user connection. Listen to user.from to catch this.
func (user *User) close(code websocket.StatusCode, reason string) {
	fmt.Println("User: closing")
	close(user.from)
	close(user.to)
	user.connection.Close(code, reason)
}

func (user *User) startReading() {
	for {
		// Read JSON incomming JSON events
		message := Message{}
		err := wsjson.Read(context.Background(), user.connection, &message)
		// TODO: handle errors nicely
		if err != nil {
			fmt.Println(fmt.Errorf("User JSON WS read failed: %w", err))
			user.close(websocket.StatusInternalError, "Client read failure")
			return
		}
		user.from <- message
	}
}

/* Listen on the message to user channel and write all data out to the client. */
func (user *User) startListening() {
	for {
		select {
		case data, ok := <-user.to:
			// Channel closed, stop listening
			if !ok {
				fmt.Println("User: Listening returning")
				return
			}
			fmt.Println("User: writing JSON data")
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*6)
			wsjson.Write(ctx, user.connection, &data)
			cancel()
		}
	}
}
