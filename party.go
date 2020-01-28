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

// NewParty creates a new room for users to join.
func NewParty(name string, incoming chan Incoming, options *Options) *Party {
	return &Party{
		Name:     name,
		opts:     options,
		Incoming: incoming,

		UserAddedHandler:   func(u uuid.UUID) {},
		UserRemovedHandler: func(u uuid.UUID) {},
		ErrorHandler:       func(e error) {},

		connectedUsers: make(map[uuid.UUID]*User),
	}
}

// Party represents a group of users connected in a socket session.
type Party struct {
	// Human readable name of the party
	Name string

	// Receive messages
	Incoming chan Incoming

	// Called when an error occurs within the party.
	ErrorHandler func(err error)

	// Called when a user joins the party.
	UserAddedHandler func(userID uuid.UUID)
	// Called when a user has left the party. The user is already gone, messages to them will not be sent.
	UserRemovedHandler func(userID uuid.UUID)

	opts           *Options
	connectedUsers map[uuid.UUID]*User
	mut            sync.Mutex
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

	// Generate a new user
	user, err := newUser(
		party.Incoming,
		conn,
		party.opts,
	)

	if err != nil {
		go party.ErrorHandler(fmt.Errorf("Failed to create new user: %v", err))
		conn.Close(websocket.StatusInternalError, "User creation failure.")
		return
	}

	// Add the user and begin processing
	party.addUser(user)
	userClosed := make(chan error)
	go user.Listen(req.Context(), userClosed)
	for {
		select {
		case err := <-userClosed:
			// User listen closed
			if err != nil {
				go party.ErrorHandler(err)
			}
			party.removeUser(user.ID)
			return
		}
	}
}

// GetConnectedUserCount returns the number of currently connected users.
func (party *Party) GetConnectedUserCount() int {
	party.mut.Lock()
	defer party.mut.Unlock()
	return len(party.connectedUsers)
}

// SendMessage routes a message to the appropraite connected users.
func (party *Party) SendMessage(ctx context.Context, message *Outgoing) {
	if message.Broadcast {
		party.broadcast(ctx, message)
	} else {
		party.messageUser(ctx, message)
	}
}

// End closes all user connections and remove them from the party. Not dumb, tries to close cleanly.
func (party *Party) End() {
	party.mut.Lock()
	defer party.mut.Unlock()
	for _, user := range party.connectedUsers {
		user.Close("Party ending.")
		delete(party.connectedUsers, user.ID)
	}
}

// Removethe user from the party's list. Dumb op.
func (party *Party) removeUser(id uuid.UUID) error {
	party.mut.Lock()
	if user, ok := party.connectedUsers[id]; ok {
		delete(party.connectedUsers, user.ID)
		party.mut.Unlock()
		go party.UserRemovedHandler(user.ID)
		return nil
	}
	party.mut.Unlock()
	return errors.New("No such user")
}

// Add a user to the party's list. Dumb op.
func (party *Party) addUser(user *User) {
	party.mut.Lock()
	party.connectedUsers[user.ID] = user
	party.mut.Unlock()
	go party.UserAddedHandler(user.ID)
}

// Push to all users
func (party *Party) broadcast(ctx context.Context, message *Outgoing) {
	party.mut.Lock()
	defer party.mut.Unlock()
	for _, user := range party.connectedUsers {
		user.SendOutgoing(ctx, message)
	}
}

// Push to one user
func (party *Party) messageUser(ctx context.Context, message *Outgoing) {
	party.mut.Lock()
	defer party.mut.Unlock()
	if user, ok := party.connectedUsers[message.UserID]; ok {
		user.SendOutgoing(ctx, message)
	}
}
