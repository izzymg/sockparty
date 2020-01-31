package sockparty

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/google/uuid"
	"nhooyr.io/websocket"
)

// ErrNoSuchUser is returned when an invalid user is looked up.
var ErrNoSuchUser = errors.New("No such user found by that ID")

// New creates a new room for users to join.
func New(name string, incoming chan Incoming, joined chan<- uuid.UUID, left chan<- uuid.UUID, options *Options) *Party {
	return &Party{
		Name:     name,
		Incoming: incoming,
		Joined:   joined,
		Left:     left,

		ErrorHandler: func(e error) {},

		opts:           options,
		connectedUsers: make(map[uuid.UUID]*user),
	}
}

// Party represents a group of users connected in a socket session.
type Party struct {
	// Human readable name of the party
	Name string

	// Receive messages
	Incoming chan Incoming
	Joined   chan<- uuid.UUID
	Left     chan<- uuid.UUID

	// Called when an error occurs within the party.
	ErrorHandler func(err error)

	opts           *Options
	connectedUsers map[uuid.UUID]*user
	mut            sync.RWMutex
}

/*
ServeHTTP handles an HTTP request to join the room and upgrade to WebSocket.
It blocks until the user leaves/disconnects.
*/
func (party *Party) ServeHTTP(rw http.ResponseWriter, req *http.Request) {

	// Upgrade the HTTP request to a socket connection
	conn, err := websocket.Accept(rw, req, &websocket.AcceptOptions{
		InsecureSkipVerify: party.opts.AllowCrossOrigin,
	})
	if err != nil {
		party.ErrorHandler(fmt.Errorf("Failed to upgrade websocket connection: %v", err))
		return
	}

	/* Party's incoming channel is passed to new users, so all incoming data
	is funnelled back to the consumer. */
	usr, err := newUser(
		party.Incoming,
		conn,
		party.opts,
	)

	if err != nil {
		party.ErrorHandler(fmt.Errorf("Failed to create new user: %v", err))
		conn.Close(websocket.StatusInternalError, "User creation failure.")
		return
	}

	// Add the user and begin processing
	party.addUser(usr)
	closed := make(chan error)
	go usr.listen(req.Context(), closed)
	for {
		select {
		case err := <-closed:
			// User listen closed
			if err != nil {
				go party.ErrorHandler(err)
			}
			party.removeUser(usr.ID)
			return
		}
	}
}

// GetConnectedUserCount returns the number of currently connected users.
func (party *Party) GetConnectedUserCount() int {
	party.mut.RLock()
	defer party.mut.RUnlock()
	return len(party.connectedUsers)
}

// Broadcast writes a single outgoing message to all users currently active in the party.
func (party *Party) Broadcast(ctx context.Context, message *Outgoing) error {
	party.mut.RLock()
	defer party.mut.RUnlock()
	for _, usr := range party.connectedUsers {
		usr.write(ctx, message)
	}
	return nil
}

// Message writes a single outgoing message to a user by their ID.
func (party *Party) Message(ctx context.Context, userID uuid.UUID, message *Outgoing) error {
	party.mut.RLock()
	defer party.mut.RUnlock()
	if usr, ok := party.connectedUsers[userID]; ok {
		return usr.write(ctx, message)
	}
	return ErrNoSuchUser
}

// End closes all user connections and remove them from the party. Not dumb, tries to close cleanly.
func (party *Party) End() {
	party.mut.Lock()
	defer party.mut.Unlock()
	for _, user := range party.connectedUsers {
		user.close("Party ending.")
		delete(party.connectedUsers, user.ID)
	}
}

// Removethe user from the party's list. Dumb op.
func (party *Party) removeUser(id uuid.UUID) error {
	party.mut.Lock()
	defer party.mut.Unlock()
	if user, ok := party.connectedUsers[id]; ok {
		delete(party.connectedUsers, user.ID)
		party.userEvent(false, id)
		return nil
	}
	return ErrNoSuchUser
}

// Add a user to the party's list. Dumb op.
func (party *Party) addUser(usr *user) {
	party.mut.Lock()
	defer party.mut.Unlock()
	party.connectedUsers[usr.ID] = usr
	party.userEvent(true, usr.ID)
}

// Send to user join/leave channels without blocking
func (party *Party) userEvent(join bool, id uuid.UUID) {
	c := party.Joined
	if !join {
		c = party.Left
	}
	select {
	case c <- id:
	default:
	}
}
