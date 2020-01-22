package sockroom

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"golang.org/x/time/rate"
	"nhooyr.io/websocket"
)

// NewParty creates a new room for users to join.
func NewParty(name string, options *Options) *Party {
	return &Party{
		Name:           name,
		Options:        options,
		connectedUsers: make(map[string]*User),
	}
}

// Options configures a party's settings.
type Options struct {
	// The origin header that must be present for users to connect.
	AllowedOrigin string
	// Limiter used against incoming client messages.
	RateLimiter *rate.Limiter
}

// Party represents a group of users connected in a socket session.
type Party struct {
	Name           string
	Options        *Options
	connectedUsers map[string]*User
	mut            sync.Mutex
}

// ServeHTTP handles an HTTP request to join the room and upgrade to WebSocket.
func (party *Party) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	fmt.Println("Party: got request")

	// Upgrade the HTTP request to a socket connection
	conn, err := websocket.Accept(rw, req, &websocket.AcceptOptions{})
	if err != nil {
		fmt.Printf("Party: Failed to upgrade websocket connection: %v", err)
		return
	}

	// Add the user to the party
	user, err := newUser(
		fmt.Sprintf("User %d", party.GetConnectedUserCount()),
		conn,
	)
	if err != nil {
		fmt.Printf("Party: Failed to create new user: %v", err)
		conn.Close(websocket.StatusInternalError, "User creation failure.")
		return
	}
	party.addUser(user)
	err = party.processUser(req.Context(), user.ID)
	if err != nil {
		fmt.Println(err)
	}
}

// GetConnectedUserCount returns the number of currently connected users.
func (party *Party) GetConnectedUserCount() int {
	return len(party.connectedUsers)
}

// Adds a new user to the room. Dumb op.
func (party *Party) addUser(user *User) {
	fmt.Printf("Party: adding user %s\n", user.ID)
	// Add the connected user
	party.mut.Lock()
	defer party.mut.Unlock()
	party.connectedUsers[user.ID] = user
}

// Grabs a user by their ID, or nil if they don't exist.
func (party *Party) getUser(id string) *User {
	fmt.Printf("Party: fetching user %s\n", id)
	party.mut.Lock()
	defer party.mut.Unlock()
	return party.connectedUsers[id]
}

// Removes a user from the list of connected users. Performs no cleanup - dumb op.
func (party *Party) deleteUser(id string) {
	fmt.Printf("Party: deleting user %s\n", id)
	party.mut.Lock()
	defer party.mut.Unlock()
	delete(party.connectedUsers, id)
}

/* Process user begins processing the user's requests.
If the HTTP request is canceled for any reason, the context will go with it,
cascading and cleaning up. This function blocks, waiting for any messages from the user
or the user to close before deleting it and returning. */
func (party *Party) processUser(ctx context.Context, id string) error {

	// Grab user by ID
	user := party.getUser(id)
	if user == nil {
		return fmt.Errorf("Failed to find user by ID key %v", id)
	}

	// Begin processing incoming and outgoing data.
	go user.listenIncoming(ctx, party.Options.RateLimiter)
	go user.listenOutgoing(ctx)

	for {
		select {
		case err := <-user.closed:
			party.deleteUser(user.ID)
			return fmt.Errorf("Party: process user closed: %w", err)
		case message := <-user.fromUser:
			fmt.Printf("Party: message %v\n", message)
			party.broadcast(message)
		}
	}
}

// Write a message out to a user in the party by their ID.
func (party *Party) messageUser(id string, message Message) error {
	fmt.Printf("Party: messaging %s\n", id)
	user := party.getUser(id)
	if user == nil {
		return fmt.Errorf("Message user failed, no such user %s", id)
	}

	// Don't block if the user isn't available.
	// TODO: Timeout here.
	select {
	case user.toUser <- message:
		fmt.Println("Party: Message user successful")
	default:
		fmt.Println("Party: Message user skipped, no receiver")
	}
	return nil
}

// Iterates over all connected users in this party and sends data to them.
func (party *Party) broadcast(message Message) {
	fmt.Println("Party: broadcasting")
	party.mut.Lock()
	defer party.mut.Unlock()
	for _, user := range party.connectedUsers {
		select {
		// Don't block if the user isn't available.
		// TODO: Timeout here.
		case user.toUser <- message:
			fmt.Println("Party: broadcast successful")
		default:
			fmt.Println("Party: broadcast skipped, no receiver")
		}
	}
}
