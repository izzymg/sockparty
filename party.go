package sockparty

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"nhooyr.io/websocket"
)

// NewParty creates a new room for users to join.
func NewParty(name string, incoming chan IncomingMessage, outgoing chan OutgoingMessage, options *Options) *Party {
	return &Party{
		Name:        name,
		Options:     options,
		SendMessage: outgoing,
		Incoming:    incoming,

		UserAddedHandler:          func(p *Party, u *User) {},
		UserRemovedHandler:        func(p *Party, u *User) {},
		UserInvalidMessageHandler: func(p *Party, u *User, m IncomingMessage) {},
		ErrorHandler:              func(e error) {},

		connectedUsers: make(map[string]*User),
	}
}

// Party represents a group of users connected in a socket session.
type Party struct {
	// Human readable name of the party
	Name string
	// Configuration options for the party
	Options *Options
	// Send an outgoing message to users
	SendMessage chan OutgoingMessage
	// Receive messages
	Incoming chan IncomingMessage

	ErrorHandler              func(err error)
	UserAddedHandler          func(party *Party, user *User)
	UserRemovedHandler        func(party *Party, user *User)
	UserInvalidMessageHandler func(party *Party, user *User, message IncomingMessage)

	// Connections currently active in this party
	connectedUsers map[string]*User
	mut            sync.Mutex

	messageHandlers map[MessageEvent]MessageHandler
}

/*ServeHTTP handles an HTTP request to join the room and upgrade to WebSocket. As this adds users to the party,
it will block indefinitely if the party is not currently listening. */
func (party *Party) ServeHTTP(rw http.ResponseWriter, req *http.Request) {

	// Upgrade the HTTP request to a socket connection
	conn, err := websocket.Accept(rw, req, &websocket.AcceptOptions{
		InsecureSkipVerify: party.Options.AllowCrossOrigin,
	})
	if err != nil {
		party.ErrorHandler(fmt.Errorf("Failed to upgrade websocket connection: %v", err))
		return
	}

	// Generate a new user
	user, err := newUser(
		party.Incoming,
		conn,
		party.Options,
	)

	if err != nil {
		go party.ErrorHandler(fmt.Errorf("Failed to create new user: %v", err))
		conn.Close(websocket.StatusInternalError, "User creation failure.")
		return
	}

	// Add the user and begin processing
	party.addUser(user)

	closed := make(chan error)
	go user.Listen(req.Context(), closed)
	for {
		select {
		case err := <-closed:
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
	return len(party.connectedUsers)
}

// Removethe user from the party's list. Dumb op.
func (party *Party) removeUser(id string) error {
	party.mut.Lock()
	defer party.mut.Unlock()
	if user, ok := party.connectedUsers[id]; ok {
		delete(party.connectedUsers, user.ID)
		return nil
	}
	return errors.New("No such user")
}

// Add a user to the party's list. Dumb op.
func (party *Party) addUser(user *User) {
	party.mut.Lock()
	defer party.mut.Unlock()
	party.connectedUsers[user.ID] = user
}

// Close all user connections and remove them from the party. Not dumb, tries to close cleanly.
func (party *Party) removeUsers() {
	party.mut.Lock()
	defer party.mut.Unlock()
	for _, user := range party.connectedUsers {
		user.Close("Party ending.")
		delete(party.connectedUsers, user.ID)
	}
}

// Push to all users
func (party *Party) broadcast(ctx context.Context, message *OutgoingMessage) {
	party.mut.Lock()
	defer party.mut.Unlock()
	for _, user := range party.connectedUsers {
		message.UserID = user.ID
		party.messageUser(ctx, message)
	}
}

// Push to one user
func (party *Party) messageUser(ctx context.Context, message *OutgoingMessage) {
	party.mut.Lock()
	defer party.mut.Unlock()
	if user, ok := party.connectedUsers[message.UserID]; ok {
		user.SendOutgoing(ctx, message)
	}
}
