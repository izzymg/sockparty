package sockparty

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"golang.org/x/time/rate"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const (
	timeout    = "Connection timed out."
	disconnect = "Disconnected."
)

// newUser creates a new user from a websocket connection. Generates it a new unique ID for lookups.
func newUser(id string, incoming chan Incoming, connection *websocket.Conn, opts *Options) *user {
	return &user{
		ID:         id,
		incoming:   incoming,
		opts:       opts,
		connection: connection,
	}
}

// user represents a websocket connection from a client.
type user struct {
	ID         string
	Name       string
	opts       *Options
	connection *websocket.Conn
	incoming   chan Incoming
}

/*
Listen begins processing the user's connection, sending information back to
its given channels. This routine blocks.
*/
func (usr *user) listen(ctx context.Context, closed chan error) {
	// Cancel context when one routine exits, causing a cascade cleanup.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	/* Don't block on closed channel if no one is listening. */
	go func() {
		defer cancel()
		select {
		case closed <- usr.handleIncoming(ctx):
		default:
		}
	}()

	select {
	case closed <- usr.handleLifecycle(ctx):
	default:
	}
}

/* Handle pings to the user, and drop when the context is canceled. */
func (usr *user) handleLifecycle(ctx context.Context) error {
	var ticker *time.Ticker
	// Don't ping, ugly
	if usr.opts.PingFrequency > 0 {
		ticker = time.NewTicker(usr.opts.PingFrequency)
	} else {
		ticker = time.NewTicker(time.Second)
		ticker.Stop()
	}

	for {
		select {
		case <-ctx.Done():
			// Context dropped (Upgraded request may have been killed)
			usr.close(timeout)
			return ctx.Err()
		case <-ticker.C:
			// Ping the user and wait for a pong back. Assume dead if no response.
			err := usr.ping(ctx)
			if err != nil {
				usr.close("Disconnected.")
				return err
			}
		}
	}
}

/* Listen on all incoming JSON messages from the client, writing them into the users'
incoming channel. Will die if the context is canceled or read message fails. */
func (usr *user) handleIncoming(ctx context.Context) error {

	limiter := usr.opts.RateLimiter
	if limiter == nil {
		limiter = rate.NewLimiter(rate.Inf, 1)
	}

	// TODO: implement proper error structures and close at the return.
	for {
		// Context canceled, cleanup the connection
		select {
		case <-ctx.Done():
			usr.close(timeout)
			return ctx.Err()
		default:
		}
		// Wait for the limiter
		err := limiter.Wait(ctx)
		if err != nil {
			usr.close(timeout)
			return err
		}
		// Read any JSON.
		message, err := usr.read(ctx)
		if err != nil {
			usr.close(disconnect)
			return err
		}
		if usr.incoming != nil {
			usr.incoming <- *message
		}
	}
}

// close ends the users connection, causing a cascade cleanup.
func (usr *user) close(reason string) error {
	err := usr.connection.Close(websocket.StatusNormalClosure, reason)
	if err != nil {
		return fmt.Errorf("Closing user connection failed: %w", err)
	}
	return nil
}

// write sends a message to the user.
func (usr *user) write(ctx context.Context, message *Outgoing) error {
	err := wsjson.Write(ctx, usr.connection, message)
	if err != nil {
		return fmt.Errorf("Write JSON to user failed: %w", err)
	}
	return nil
}

// Blocks until a message comes through from the connection and reads it.
func (usr *user) read(ctx context.Context) (*Incoming, error) {

	var payload json.RawMessage
	im := &Incoming{
		UserID:  usr.ID,
		Payload: payload,
	}

	err := wsjson.Read(ctx, usr.connection, im)
	if err != nil {
		return nil, fmt.Errorf("Read JSON from user failed: %w", err)
	}

	return im, nil
}

// Blocks until user responds with a pong/context cancels
func (usr *user) ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, usr.opts.PingTimeout)
	defer cancel()
	err := usr.connection.Ping(ctx)
	if err != nil {
		return fmt.Errorf("Ping failed: %w", err)
	}
	return nil
}
