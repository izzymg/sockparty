package sockparty

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"

	"github.com/google/uuid"
)

// Create a new user from a websocket connection. Generates it a new unique ID for lookups.
func newUser(name string, connection *websocket.Conn, options *Options) (*User, error) {
	uid, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("Failed to generate a UUID for a new user: %w", err)
	}
	return &User{
		Name:       name,
		ID:         uid.String(),
		options:    options,
		connection: connection,
		closed:     make(chan error),
		// TODO: add buffers here
		incoming: make(chan IncomingMessage),
	}, nil
}

// User represents a websocket connection from a client.
type User struct {
	ID         string
	Name       string
	options    *Options
	connection *websocket.Conn
	closed     chan error
	incoming   chan IncomingMessage
}

// Close ends the users connection, causing a cascade cleanup.
func (user *User) Close(reason string) {
	user.connection.Close(websocket.StatusNormalClosure, reason)
}

// SendOutgoing sends a message to the user.
func (user *User) SendOutgoing(ctx context.Context, message *OutgoingMessage) error {
	err := wsjson.Write(ctx, user.connection, message)
	if err != nil {
		return fmt.Errorf("Write JSON to user failed: %w", err)
	}
	return nil
}

// Blocks until a message comes through from the connection and reads it.
func (user *User) read(ctx context.Context) (*IncomingMessage, error) {

	var payload json.RawMessage
	im := &IncomingMessage{
		UserID:  user.ID,
		Payload: payload,
	}

	err := wsjson.Read(ctx, user.connection, im)
	if err != nil {
		return nil, fmt.Errorf("Read JSON from user failed: %w", err)
	}

	return im, nil
}

// Blocks until user responds with a pong/context cancels
func (user *User) ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, user.options.PingTimeout)
	defer cancel()
	err := user.connection.Ping(ctx)
	if err != nil {
		return fmt.Errorf("Ping failed: %w", err)
	}
	return nil
}

/* Handle pings to the user, and drop when the context is canceled. */
func (user *User) handleLifecycle(ctx context.Context) {
	var ticker *time.Ticker
	// Don't ping, ugly
	if user.options.PingFrequency > 0 {
		ticker = time.NewTicker(user.options.PingFrequency)
	} else {
		ticker = time.NewTicker(time.Second)
		ticker.Stop()
	}

	for {
		select {
		case <-ctx.Done():
			// Context dropped (Upgraded request may have been killed)
			user.Close("Connection timed out.")
			return
		case <-ticker.C:
			// Ping the user and wait for a pong back. Assume dead if no response.
			err := user.ping(ctx)
			if err != nil {
				user.Close("Disconnected.")
				return
			}
		}
	}
}

/* Listen on all incoming JSON messages from the client, writing them into the users'
fromUser channel. Will die if the context is canceled or read message fails. */
func (user *User) handleIncoming(ctx context.Context) {

	limiter := user.options.RateLimiter
	if limiter == nil {
		limiter = rate.NewLimiter(rate.Inf, 1)
	}

	for {
		// Make sure the context isn't dead
		select {
		case <-ctx.Done():
			user.Close("Connection timed out.")
			return
		default:
		}
		// Wait for the limiter
		err := limiter.Wait(ctx)
		if err != nil {
			continue
		}
		// Read any JSON
		message, err := user.read(ctx)
		if err != nil {
			user.closed <- err
			return
		}
		user.incoming <- *message
	}
}
