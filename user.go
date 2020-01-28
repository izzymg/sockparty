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

const (
	timeout    = "Connection timed out."
	disconnect = "Disconnected."
)

// Create a new user from a websocket connection. Generates it a new unique ID for lookups.
func newUser(incoming chan IncomingMessage, connection *websocket.Conn, options *Options) (*User, error) {
	uid, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("Failed to generate a UUID for a new user: %w", err)
	}
	return &User{
		ID:         uid.String(),
		incoming:   incoming,
		options:    options,
		connection: connection,
	}, nil
}

// User represents a websocket connection from a client.
type User struct {
	// TODO: UUID
	ID         string
	Name       string
	options    *Options
	connection *websocket.Conn
	incoming   chan IncomingMessage
}

/*
Listen begins processing the user's connection, sending information back to
its given channels. This routine blocks.
*/
func (user *User) Listen(ctx context.Context, closed chan error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// TODO: remove prints

	// Cancel context when one routine exits, causing a cascade cleanup.
	go func() {
		defer cancel()
		select {
		case closed <- user.handleIncoming(ctx):
		default:
		}

		fmt.Println("Incoming returned")
	}()
	select {
	case closed <- user.handleLifecycle(ctx):
	default:
	}
	fmt.Println("Lifecycle returned")
}

// Close ends the users connection, causing a cascade cleanup.
func (user *User) Close(reason string) error {
	err := user.connection.Close(websocket.StatusNormalClosure, reason)
	if err != nil {
		return fmt.Errorf("Closing user connection failed: %w", err)
	}
	return nil
}

// SendOutgoing sends a message to the user.
func (user *User) SendOutgoing(ctx context.Context, message *OutgoingMessage) error {
	err := wsjson.Write(ctx, user.connection, message)
	if err != nil {
		return fmt.Errorf("Write JSON to user failed: %w", err)
	}
	return nil
}

/* Handle pings to the user, and drop when the context is canceled. */
func (user *User) handleLifecycle(ctx context.Context) error {
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
			user.Close(timeout)
			return ctx.Err()
		case <-ticker.C:
			// Ping the user and wait for a pong back. Assume dead if no response.
			err := user.ping(ctx)
			if err != nil {
				user.Close("Disconnected.")
				return err
			}
		}
	}
}

/* Listen on all incoming JSON messages from the client, writing them into the users'
incoming channel. Will die if the context is canceled or read message fails. */
func (user *User) handleIncoming(ctx context.Context) error {

	limiter := user.options.RateLimiter
	if limiter == nil {
		limiter = rate.NewLimiter(rate.Inf, 1)
	}

	// TODO: implement proper error structures and close at the return.
	for {
		// Context canceled, cleanup the connection
		select {
		case <-ctx.Done():
			user.Close(timeout)
			return ctx.Err()
		default:
		}
		// Wait for the limiter
		err := limiter.Wait(ctx)
		if err != nil {
			user.Close(timeout)
			return err
		}
		// Read any JSON.
		message, err := user.read(ctx)
		if err != nil {
			user.Close(disconnect)
			return err
		}
		user.incoming <- *message
	}
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
