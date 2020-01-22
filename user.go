package sockparty

import (
	"context"
	"fmt"

	"golang.org/x/time/rate"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/google/uuid"
)

// Message is a struct needing documentation and a new home.
type Message struct {
	Event       string      `json:"event"`
	Destination string      `json:"destination"`
	Payload     interface{} `json:"payload"`
}

// Create a new user from a websocket connection. Generates it a new unique ID for lookups.
func newUser(name string, connection *websocket.Conn) (*User, error) {
	uid, err := uuid.NewUUID()
	if err != nil {
		return nil, fmt.Errorf("Failed to generate a UUID for a new user: %w", err)
	}
	return &User{
		Name:       name,
		ID:         uid.String(),
		connection: connection,
		closed:     make(chan error),
		// TODO: add buffers here
		fromUser: make(chan Message),
		toUser:   make(chan Message),
	}, nil
}

// User represents a websocket connection from a client.
type User struct {
	ID         string
	Name       string
	connection *websocket.Conn
	closed     chan error
	fromUser   chan Message
	toUser     chan Message
}

// Writes message to user
func (user *User) writeMessage(ctx context.Context, message *Message) error {
	err := wsjson.Write(ctx, user.connection, message)
	if err != nil {
		return fmt.Errorf("Write JSON to user failed: %w", err)
	}
	return nil
}

// Blocks until next message and returns it
func (user *User) readMessage(ctx context.Context) (*Message, error) {
	message := &Message{}
	err := wsjson.Read(ctx, user.connection, &message)
	if err != nil {
		return nil, fmt.Errorf("Read JSON from user failed: %w", err)
	}
	return message, nil
}

/* Listen on any messages destined to the user through its toUser channel,
and write them to the user. Will die if context is canceled or on write failure. */
func (user *User) listenOutgoing(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			fmt.Println("User: listen outgoing context finished")
			return
		case message := <-user.toUser:
			err := user.writeMessage(ctx, &message)
			if err != nil {
				user.connection.Close(websocket.StatusInternalError, "Write failure")
				user.closed <- err
				return
			}
		}
	}
}

/* Listen on all incoming JSON messages from the client, writing them into the users'
fromUser channel. Will die if the context is canceled or read message fails. */
func (user *User) listenIncoming(ctx context.Context, limiter *rate.Limiter) {
	if limiter == nil {
		limiter = rate.NewLimiter(rate.Inf, 1)
	}

	for {
		select {
		// Make sure the context isn't dead
		case <-ctx.Done():
			fmt.Println("User: listen incoming context finished")
			return
		default:
			// Wait for the limiter
			err := limiter.Wait(ctx)
			if err != nil {
				fmt.Println(fmt.Errorf("User rate limit error: %w", err))
				continue
			}
			// Read any JSON
			message, err := user.readMessage(ctx)
			if err != nil {
				fmt.Printf("User: listen incoming closed: %v\n", err)
				// Indicate the user is dead with an error
				// TODO: check what kind of error and handle appropriately
				user.connection.Close(websocket.StatusInternalError, "Read failure")
				user.closed <- err
				return
			}
			user.fromUser <- *message
		}
	}
}
